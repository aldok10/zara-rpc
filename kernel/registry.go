package kernel

// Service groups related operations under a service name, mirroring a
// protobuf service. The name is used to derive fully-qualified procedure
// names for operations that do not set one explicitly.
type Service struct {
	// Name is the fully-qualified service name, e.g.
	// "acme.users.v1.UsersService".
	Name string
	// Prefix is an optional URL prefix prepended to every operation path.
	// For example, a service with Prefix "/api" and an operation
	// "/v1/users/{id}" serves at "/api/v1/users/{id}".
	Prefix string

	operations []*Operation
}

// ServiceBuilder builds a Service in place with zero extra allocations.
// It is the underlying type of Service, so converting a *Service to
// *ServiceBuilder is free; setters mutate the struct and return the
// builder for chaining.
//
//	var s Service
//	(*ServiceBuilder)(&s).SetName("acme.users.v1.UsersService").Build()
type ServiceBuilder Service

// SetName sets the fully-qualified service name.
func (b *ServiceBuilder) SetName(name string) *ServiceBuilder {
	b.Name = name
	return b
}

// SetPrefix sets the URL prefix prepended to every operation path.
func (b *ServiceBuilder) SetPrefix(prefix string) *ServiceBuilder {
	b.Prefix = prefix
	return b
}

// Add registers an operation on the service.
func (b *ServiceBuilder) Add(ops ...*Operation) *ServiceBuilder {
	b.operations = append(b.operations, ops...)
	return b
}

// Build finalizes the Service and returns its pointer.
func (b *ServiceBuilder) Build() *Service {
	return (*Service)(b)
}

// NewService creates a service with the given fully-qualified name
// and registers the provided operations.
func NewService(name string, ops ...*Operation) *Service {
	var s Service
	return (*ServiceBuilder)(&s).
		SetName(name).
		Add(ops...).
		Build()
}

// Add registers an operation on the service and returns the
// service for chaining.
func (s *Service) Add(ops ...*Operation) *Service {
	s.operations = append(s.operations, ops...)
	return s
}

// Operations returns the registered operations.
func (s *Service) Operations() []*Operation {
	return s.operations
}
