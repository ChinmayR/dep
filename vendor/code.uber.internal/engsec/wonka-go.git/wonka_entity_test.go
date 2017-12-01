package wonka_test

import (
	"strings"
	"testing"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"github.com/stretchr/testify/require"
)

func TestEntityName(t *testing.T) {
	e := wonka.Entity{EntityName: "FooBErDooBer"}
	require.Equal(t, e.Name(), strings.ToLower(e.EntityName))

	require.Equal(t, wonka.CanonicalEntityName(e.EntityName),
		"fooberdoober")
}
