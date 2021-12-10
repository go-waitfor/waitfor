package waitfor

type Module func() ([]string, ResourceFactory)

func Use(mod Module) ResourceConfig {
	scheme, factory := mod()

	return ResourceConfig{
		Scheme:  scheme,
		Factory: factory,
	}
}
