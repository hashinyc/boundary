package controller

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/hashicorp/boundary/internal/cmd/base"
	pbs "github.com/hashicorp/boundary/internal/gen/controller/servers/services"
	"github.com/hashicorp/boundary/internal/libs/alpnmux"
	"github.com/hashicorp/boundary/internal/servers/controller/handlers/workers"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/go-secure-stdlib/strutil"
	"google.golang.org/grpc"
)

func (c *Controller) startListeners(ctx context.Context) error {
	servers := make([]func(), 0, len(c.conf.Listeners))

	var foundApi bool
	for _, ln := range c.conf.Listeners {
		if strutil.StrListContains(ln.Config.Purpose, "api") {
			foundApi = true
		}
	}

	if foundApi {
		grpcServer, gwTicket, err := newGrpcServer(ctx, c.IamRepoFn, c.AuthTokenRepoFn, c.ServersRepoFn, c.kms, c.conf.Eventer)
		if err != nil {
			return fmt.Errorf("failed to create new grpc server: %w", err)
		}
		c.grpcGatewayTicket = gwTicket

		err = c.registerGrpcServices(ctx, grpcServer)
		if err != nil {
			return fmt.Errorf("failed to register grpc services: %w", err)
		}

		c.grpcServerListener, _ = newGrpcServerListener()
		servers = append(servers, func() {
			go c.grpcServer.Serve(c.grpcServerListener)
		})
	}

	for i := range c.conf.Listeners {
		ln := c.conf.Listeners[i]
		for _, purpose := range ln.Config.Purpose {
			switch purpose {
			case "api":
				apiServers, err := c.configureForAPI(ctx, ln)
				if err != nil {
					return fmt.Errorf("failed to configure listener for api mode: %w", err)
				}
				servers = append(servers, apiServers...)

			case "cluster":
				err := c.configureForCluster(ctx, ln)
				if err != nil {
					return fmt.Errorf("failed to configure listener for cluster mode: %w", err)
				}
				servers = append(servers, func() { go ln.GrpcServer.Serve(ln.ALPNListener) })

			case "proxy": // In dev mode we might see this but we don't handle it here. Do nothing.
			default:
				return fmt.Errorf("unknown listener purpose %q", purpose)
			}
		}
	}

	for _, s := range servers {
		s()
	}

	return nil
}

func (c *Controller) configureForAPI(ctx context.Context, ln *base.ServerListener) ([]func(), error) {
	apiServers := make([]func(), 0)

	handler, err := c.apiHandler(HandlerProperties{
		ListenerConfig: ln.Config,
		CancelCtx:      ctx,
	})
	if err != nil {
		return nil, err
	}

	cancelCtx := c.baseContext // Resolve to avoid race conditions if the base context is replaced.
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       5 * time.Minute,
		ErrorLog:          c.logger.StandardLogger(nil),
		BaseContext:       func(net.Listener) context.Context { return cancelCtx },
	}
	ln.HTTPServer = server

	if ln.Config.HTTPReadHeaderTimeout > 0 {
		server.ReadHeaderTimeout = ln.Config.HTTPReadHeaderTimeout
	}
	if ln.Config.HTTPReadTimeout > 0 {
		server.ReadTimeout = ln.Config.HTTPReadTimeout
	}
	if ln.Config.HTTPWriteTimeout > 0 {
		server.WriteTimeout = ln.Config.HTTPWriteTimeout
	}
	if ln.Config.HTTPIdleTimeout > 0 {
		server.IdleTimeout = ln.Config.HTTPIdleTimeout
	}

	switch ln.Config.TLSDisable {
	case true:
		l, err := ln.Mux.RegisterProto(alpnmux.NoProto, nil)
		if err != nil {
			return nil, fmt.Errorf("error getting non-tls listener: %w", err)
		}
		if l == nil {
			return nil, errors.New("could not get non-tls listener")
		}
		apiServers = append(apiServers, func() { go server.Serve(l) })

	default:
		for _, v := range []string{"", "http/1.1", "h2"} {
			l := ln.Mux.GetListener(v)
			if l == nil {
				return nil, fmt.Errorf("could not get tls proto %q listener", v)
			}
			apiServers = append(apiServers, func() { go server.Serve(l) })
		}
	}

	return apiServers, nil
}

func (c *Controller) configureForCluster(ctx context.Context, ln *base.ServerListener) error {
	// Clear out in case this is a second start of the controller
	ln.Mux.UnregisterProto(alpnmux.DefaultProto)
	l, err := ln.Mux.RegisterProto(alpnmux.DefaultProto, &tls.Config{
		GetConfigForClient: c.validateWorkerTls,
	})
	if err != nil {
		return fmt.Errorf("error getting sub-listener for worker proto: %w", err)
	}

	workerReqInterceptor, err := workerRequestInfoInterceptor(ctx, c.conf.Eventer)
	if err != nil {
		return fmt.Errorf("error getting sub-listener for worker proto: %w", err)
	}

	workerServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(math.MaxInt32),
		grpc.MaxSendMsgSize(math.MaxInt32),
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				workerReqInterceptor,
				auditRequestInterceptor(ctx),  // before we get started, audit the request
				auditResponseInterceptor(ctx), // as we finish, audit the response
			),
		),
	)

	workerService := workers.NewWorkerServiceServer(c.ServersRepoFn, c.SessionRepoFn, c.workerStatusUpdateTimes, c.kms)
	pbs.RegisterServerCoordinationServiceServer(workerServer, workerService)
	pbs.RegisterSessionServiceServer(workerServer, workerService)

	interceptor := newInterceptingListener(c, l)
	ln.ALPNListener = interceptor
	ln.GrpcServer = workerServer

	return nil
}

func (c *Controller) stopListeners(serversOnly bool) error {
	wg := new(sync.WaitGroup)

	for i := range c.conf.Listeners {
		ln := c.conf.Listeners[i]
		if ln.GrpcServer != nil { // Stop Cluster gRPC Servers if any have been started.
			wg.Add(1)
			go func() {
				ln.GrpcServer.GracefulStop()
				wg.Done()
			}()
		}
		if ln.HTTPServer != nil { // Stop HTTP Servers fronting the gRPC Server w/ gRPC Gateway if any have been started.
			wg.Add(1)
			go func() {
				ctx, cancel := context.WithTimeout(c.baseContext, ln.Config.MaxRequestDuration)
				ln.HTTPServer.Shutdown(ctx)
				cancel()
				wg.Done()
			}()
		}
	}

	if c.grpcServer != nil { // Stop API gRPC Server if it has been started (Only one)
		wg.Add(1)
		go func() {
			c.grpcServer.GracefulStop()
			wg.Done()
		}()
	}

	wg.Wait()
	if serversOnly {
		return nil
	}

	// Stop all listeners
	var retErr *multierror.Error
	for _, ln := range c.conf.Listeners {
		if err := ln.Mux.Close(); err != nil {
			if _, ok := err.(*os.PathError); ok && ln.Config.Type == "unix" {
				// The rmListener probably tried to remove the file but it
				// didn't exist, ignore the error; this is a conflict
				// between rmListener and the default Go behavior of
				// removing auto-vivified Unix domain sockets.
			} else {
				retErr = multierror.Append(retErr, err)
			}
		}
	}

	return retErr.ErrorOrNil()
}
