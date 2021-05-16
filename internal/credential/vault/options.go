package vault

// getOpts - iterate the inbound Options and return a struct
func getOpts(opt ...Option) options {
	opts := getDefaultOptions()
	for _, o := range opt {
		o(&opts)
	}
	return opts
}

// Option - how Options are passed as arguments.
type Option func(*options)

// options = how options are represented
type options struct {
	withName          string
	withDescription   string
	withLimit         int
	withCACert        []byte
	withNamespace     string
	withTlsServerName string
	withTlsSkipVerify bool
	withClientCert    *ClientCertificate
	withMethod        Method
	withRequestBody   []byte
}

func getDefaultOptions() options {
	return options{
		withMethod: MethodGet,
	}
}

// WithDescription provides an optional description.
func WithDescription(desc string) Option {
	return func(o *options) {
		o.withDescription = desc
	}
}

// WithName provides an optional name.
func WithName(name string) Option {
	return func(o *options) {
		o.withName = name
	}
}

// WithLimit provides an option to provide a limit. Intentionally allowing
// negative integers. If WithLimit < 0, then unlimited results are
// returned. If WithLimit == 0, then default limits are used for results.
func WithLimit(l int) Option {
	return func(o *options) {
		o.withLimit = l
	}
}

// WithCACert provides an optional PEM-encoded certificate
// to verify the Vault server's SSL certificate.
func WithCACert(cert []byte) Option {
	return func(o *options) {
		o.withCACert = cert
	}
}

// WithNamespace provides an optional Vault namespace.
func WithNamespace(namespace string) Option {
	return func(o *options) {
		o.withNamespace = namespace
	}
}

// WithTlsServerName provides an optional name to use as the SNI host when
// connecting to Vault via TLS.
func WithTlsServerName(name string) Option {
	return func(o *options) {
		o.withTlsServerName = name
	}
}

// WithTlsSkipVerify provides an option to disable verification of TLS
// certificates when connection to Vault. Using this option is highly
// discouraged as it decreases the security of data transmissions to and
// from the Vault server.
func WithTlsSkipVerify(skipVerify bool) Option {
	return func(o *options) {
		o.withTlsSkipVerify = skipVerify
	}
}

// WithClientCert provides an optional ClientCertificate to use for TLS
// authentication to a Vault server.
func WithClientCert(clientCert *ClientCertificate) Option {
	return func(o *options) {
		o.withClientCert = clientCert
	}
}

// WithMethod provides an optional Method to use for communicating with
// Vault.
func WithMethod(m Method) Option {
	return func(o *options) {
		o.withMethod = m
	}
}

// WithRequestBody provides an optional request body for sending to Vault
// when requesting credentials using HTTP Post.
func WithRequestBody(b []byte) Option {
	return func(o *options) {
		o.withRequestBody = b
	}
}
