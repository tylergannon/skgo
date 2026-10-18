package gen

import (
	"testing"
)

func TestMergeGoKeepsFileLocalImportsAndLocalShadows(t *testing.T) {
	t.Parallel()
	first := `package fixture
import value "strings"
func Upper(s string) string { return value.ToUpper(s) }
`
	second := `package fixture
import value "strconv"
func Number() string { return value.Itoa(42) }
func Shadow() int { value:=struct{N int}{N:7}; return value.N }
`
	merged, err := mergeGo([]string{first, second})
	if err != nil {
		t.Fatal(err)
	}
	pkg := declarationPackage(t, "fixture", "skgo_gen.go", merged, nil)
	for _, name := range []string{"Upper", "Number", "Shadow"} {
		if pkg.Types.Scope().Lookup(name) == nil {
			t.Fatalf("merge lost function %s:\n%s", name, merged)
		}
	}
}
