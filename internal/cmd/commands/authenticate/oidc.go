package authenticate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/hashicorp/boundary/api"
	"github.com/hashicorp/boundary/api/authmethods"
	"github.com/hashicorp/boundary/internal/cmd/base"
	"github.com/hashicorp/cap/util"
	"github.com/mitchellh/cli"
	"github.com/mitchellh/go-wordwrap"
	"github.com/posener/complete"
)

var (
	_ cli.Command             = (*OidcCommand)(nil)
	_ cli.CommandAutocomplete = (*OidcCommand)(nil)
)

type OidcCommand struct {
	*base.Command
}

func (c *OidcCommand) Synopsis() string {
	return wordwrap.WrapString("Invoke the OIDC auth method to authenticate with Boundary", base.TermWidth)
}

func (c *OidcCommand) Help() string {
	return base.WrapForHelpText([]string{
		"Usage: boundary authenticate oidc [options] [args]",
		"",
		"  Invoke the OIDC auth method to authenticate the Boundary CLI. Example:",
		"",
		`    $ boundary authenticate oidc -auth-method-id amoidc_1234567890`,
		"",
		"",
	}) + c.Flags().Help()
}

func (c *OidcCommand) Flags() *base.FlagSets {
	set := c.FlagSet(base.FlagSetHTTP | base.FlagSetClient | base.FlagSetOutputFormat)
	f := set.NewFlagSet("Command Options")

	f.StringVar(&base.StringVar{
		Name:   "auth-method-id",
		EnvVar: "BOUNDARY_AUTH_METHOD_ID",
		Target: &c.FlagAuthMethodId,
		Usage:  "The auth-method resource to use for the operation",
	})

	return set
}

func (c *OidcCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictAnything
}

func (c *OidcCommand) AutocompleteFlags() complete.Flags {
	return c.Flags().Completions()
}

func (c *OidcCommand) Run(args []string) int {
	f := c.Flags()

	if err := f.Parse(args); err != nil {
		c.PrintCliError(err)
		return base.CommandUserError
	}

	switch {
	case c.FlagAuthMethodId == "":
		c.PrintCliError(errors.New("Auth method ID must be provided via -auth-method-id"))
		return base.CommandUserError
	}

	client, err := c.Client(base.WithNoTokenScope(), base.WithNoTokenValue())
	if err != nil {
		c.PrintCliError(fmt.Errorf("Error creating API client: %w", err))
		return base.CommandCliError
	}
	aClient := authmethods.NewClient(client)

	result, err := aClient.Authenticate(c.Context, c.FlagAuthMethodId, "start", nil)
	if err != nil {
		if apiErr := api.AsServerError(err); apiErr != nil {
			c.PrintApiError(apiErr, "Error from controller when performing authentication start")
			return base.CommandApiError
		}
		c.PrintCliError(fmt.Errorf("Error trying to perform authentication start: %w", err))
		return base.CommandCliError
	}

	startResp := new(authmethods.OidcAuthMethodAuthenticateStartResponse)
	if err := json.Unmarshal(result.GetRawAttributes(), startResp); err != nil {
		c.PrintCliError(fmt.Errorf("Error trying to decode authenticate start response: %w", err))
		return base.CommandCliError
	}

	if base.Format(c.UI) == "table" {
		c.UI.Output("Opening returned authentication URL in your browser...")
	}
	if err := util.OpenURL(startResp.AuthUrl); err != nil {
		c.UI.Error(fmt.Errorf("Unable to open authentication URL in browser: %w", err).Error())
		c.UI.Warn("Please open the following URL manually in your web browser:")
		c.UI.Output(startResp.AuthUrl)
	}

	var watchCode int
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-c.Context.Done():
				c.PrintCliError(errors.New("Command canceled."))
				watchCode = base.CommandCliError
				return

			case <-time.After(1500 * time.Millisecond):
				result, err = aClient.Authenticate(c.Context, c.FlagAuthMethodId, "token", map[string]interface{}{
					"token_id": startResp.TokenId,
				})
				if err != nil {
					if apiErr := api.AsServerError(err); apiErr != nil {
						c.PrintApiError(apiErr, "Error from controller when performing authentication token fetch")
						watchCode = base.CommandApiError
						return
					}
					c.PrintCliError(fmt.Errorf("Error trying to perform authentication token fetch: %w", err))
					watchCode = base.CommandCliError
					return
				}
				if result.GetResponse().StatusCode() == http.StatusAccepted {
					// Nothing yet -- circle around.
					continue
				}
				return
			}
		}
	}()
	wg.Wait()

	if watchCode != 0 {
		return watchCode
	}
	if result == nil {
		c.PrintCliError(errors.New("After watching for token, no response was found."))
		return base.CommandCliError
	}

	return saveAndOrPrintToken(c.Command, result)
}
