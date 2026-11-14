package core

// InitializePolicies is a startup-only bridge, called after environment loading
// and before databases or consumers. It is not a concurrent hot-reload API.
// Registration during init does not install policies; the default is a no-op.
var InitializePolicies = func(environment map[string]string) error { return nil }
