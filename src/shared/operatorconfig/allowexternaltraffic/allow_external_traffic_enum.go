package allowexternaltraffic

import (
	"fmt"
	"github.com/spf13/pflag"
)

type Enum string

const (
	Off                 Enum = "off"
	Always              Enum = "always"
	IfBlockedByOtterize Enum = "if-blocked-by-otterize"
)

// Compile time validation for PFlag compatability
var _ pflag.Value = (*Enum)(nil)

// Set is a pointer receiver, since it shouldn't change the value of a copy
func (e *Enum) Set(value string) error {
	switch value {
	case string(Off):
		fallthrough
	case string(Always):
		fallthrough
	case string(IfBlockedByOtterize):
		*e = Enum(value)
		return nil
	}

	return fmt.Errorf("invalid value %s for allowExternalTraffic", value)
}

func (e *Enum) Type() string {
	return "allowExternalTraffic.Enum"
}

func (e *Enum) String() string {
	return string(*e)
}
