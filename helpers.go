package waitfor

// Module is a function type that returns resource configuration information.
// It provides a way for resource plugins to declare their supported URL schemes
// and factory function. This enables a plugin-like architecture where resource
// types can be developed and distributed independently.
//
// The function should return:
// - A slice of URL schemes the module supports (e.g., []string{"http", "https"})
// - A ResourceFactory function that can create resource instances from URLs
//
// Example:
//
//	func httpModule() ([]string, ResourceFactory) {
//		return []string{"http", "https"}, httpResourceFactory
//	}
type Module func() ([]string, ResourceFactory)

// Use converts a Module function into a ResourceConfig that can be used
// with New() to register resource types. This provides a convenient way
// to integrate resource plugins into a waitfor Runner.
//
// Example:
//
//	runner := waitfor.New(
//		waitfor.Use(httpModule),
//		waitfor.Use(postgresModule),
//	)
func Use(mod Module) ResourceConfig {
	scheme, factory := mod()

	return ResourceConfig{
		Scheme:  scheme,
		Factory: factory,
	}
}
