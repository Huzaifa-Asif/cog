package cog

import (
	"context"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/grafana/codejen"
	"github.com/grafana/cog/internal/testutils"
	"github.com/stretchr/testify/require"
)

func TestTypesFromSchema_OutputGo_EnumMemberNames(t *testing.T) {
	tests := []struct {
		name      string
		schema    string
		wantError string
	}{
		{
			name:      "punctuation-only member collides with enum type",
			schema:    `Foo: { baz: "foo" | "*" }`,
			wantError: `Go enum "sandbox.FooBaz": member "*" generates identifier "FooBaz", which conflicts with object "FooBaz"; set explicit, distinct enum member names`,
		},
		{
			name:      "multiple punctuation-only members",
			schema:    `Foo: { baz: "*" | "@" }`,
			wantError: `set explicit, distinct enum member names`,
		},
		{
			name:      "members normalize to the same identifier",
			schema:    `Foo: { baz: "@value" | "-value" }`,
			wantError: `Go enums "sandbox.FooBaz" and "sandbox.FooBaz": members "@value" and "-value" generate the same identifier "FooBazValue"; set explicit, distinct enum member names`,
		},
		{
			name: "members from different enums collide",
			schema: `Foo: "barBaz" | "x"
FooBar: "baz" | "y"`,
			wantError: `Go enums "sandbox.Foo" and "sandbox.FooBar": members "barBaz" and "baz" generate the same identifier "FooBarBaz"; set explicit, distinct enum member names`,
		},
		{
			name: "member collides with another enum type",
			schema: `Foo: "bar" | "baz"
FooBar: "one" | "two"`,
			wantError: `Go enum "sandbox.Foo": member "bar" generates identifier "FooBar", which conflicts with object "FooBar"; set explicit, distinct enum member names`,
		},
		{
			name:   "explicit member names preserve punctuation values",
			schema: `Foo: { baz: "foo" | "bar" | "*" | "@" | "@value" | "-value" @cog(kind="enum",memberNames="foo|bar|all|at|atValue|negateValue") }`,
		},
		{
			name: "empty and numeric member names remain supported",
			schema: `Foo: "" | "foo"
Bar: -1 | 0 | 1 @cog(kind="enum",memberNames="-1|zero|one")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cueValue := cuecontext.New().CompileString(tt.schema)
			require.NoError(t, cueValue.Err())

			files, err := TypesFromSchema().CUEValue("sandbox", cueValue).Golang(GoConfig{}).Run(context.Background())
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				require.Empty(t, files)
				return
			}
			require.NoError(t, err)
			require.Len(t, files, 1)

			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, files[0].RelativePath, files[0].Data, 0)
			require.NoError(t, err)
			config := types.Config{Importer: importer.Default()}
			_, err = config.Check("sandbox", fileSet, []*ast.File{file}, nil)
			require.NoError(t, err, "generated enums must compile")
			if tt.name == "explicit member names preserve punctuation values" {
				require.Contains(t, string(files[0].Data), `FooBazAll`)
				require.Contains(t, string(files[0].Data), `"*"`)
			}
		})
	}
}

func TestTypesFromSchema_OutputGo(t *testing.T) {
	schema := `
// Contains things.
Container: {
    str: string
}
`

	runCodegen := func(t *testing.T, config GoConfig) codejen.Files {
		t.Helper()

		req := require.New(t)

		cueValue := cuecontext.New().CompileString(schema)
		req.NoError(cueValue.Err())

		files, err := TypesFromSchema().
			CUEValue("sandbox", cueValue).
			Golang(config).
			Run(context.Background())
		req.NoError(err)

		return files
	}

	t.Run("generating go from cue - default options", func(t *testing.T) {
		req := require.New(t)

		expectedCode := `// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package sandbox

// Contains things.
type Container struct {
	Str string ` + "`json:\"str\"`" + `
}

// NewContainer creates a new Container object.
func NewContainer() *Container {
	return &Container{}
}`

		cueValue := cuecontext.New().CompileString(schema)
		req.NoError(cueValue.Err())

		files := runCodegen(t, GoConfig{})

		req.Len(files, 1, "expected a single file")
		testutils.TrimSpacesDiffComparator(t, []byte(expectedCode), files[0].Data, "test.go")
	})

	t.Run("generating go from cue - with Equal() functions", func(t *testing.T) {
		req := require.New(t)

		expectedCode := `// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package sandbox

// Contains things.
type Container struct {
	Str string ` + "`json:\"str\"`" + `
}

// NewContainer creates a new Container object.
func NewContainer() *Container {
	return &Container{}
}

// Equals tests the equality of two ` + "`Container`" + ` objects.
func (resource Container) Equals(other Container) bool {
	if resource.Str != other.Str {
		return false
	}

	return true
}`

		files := runCodegen(t, GoConfig{
			GenerateEqual: true,
		})

		req.Len(files, 1, "expected a single file")
		testutils.TrimSpacesDiffComparator(t, []byte(expectedCode), files[0].Data, "test.go")
	})
}

func TestTypesFromSchema_OutputTypescript(t *testing.T) {
	schema := `
// Contains things.
Container: {
    str: string
}
`

	expectedCode := `// Code generated - EDITING IS FUTILE. DO NOT EDIT.

/**
 * Contains things.
 */
export interface Container {
	str: string;
}

export const defaultContainer = (): Container => ({
	str: "",
});`

	t.Run("generating typescript from cue", func(t *testing.T) {
		req := require.New(t)

		cueValue := cuecontext.New().CompileString(schema)
		req.NoError(cueValue.Err())

		files, err := TypesFromSchema().
			CUEValue("sandbox", cueValue).
			Typescript(TypescriptConfig{}).
			Run(context.Background())
		req.NoError(err)

		req.Len(files, 1, "expected a single file")
		testutils.TrimSpacesDiffComparator(t, []byte(expectedCode), files[0].Data, "test.go")
	})
}

func TestTypesFromSchema_OutputOpenAPI(t *testing.T) {
	schema := `
// Contains things.
Container: {
    str: string
}
`

	expectedCode := `{
  "openapi": "3.0.0",
  "info": {
    "title": "sandbox",
    "version": "0.0.0",
    "x-schema-identifier": "",
    "x-schema-kind": ""
  },
  "paths": {},
  "components": {
    "schemas": {
      "Container": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "str"
        ],
        "properties": {
          "str": {
            "type": "string"
          }
        },
        "description": "Contains things."
      }
    }
  }
}`

	t.Run("generating openAPI from cue", func(t *testing.T) {
		req := require.New(t)

		cueValue := cuecontext.New().CompileString(schema)
		req.NoError(cueValue.Err())

		files, err := TypesFromSchema().
			CUEValue("sandbox", cueValue).
			GenerateOpenAPI(OpenAPIGenerationConfig{}).
			Run(context.Background())
		req.NoError(err)

		req.Len(files, 1, "expected a single file")
		testutils.TrimSpacesDiffComparator(t, []byte(expectedCode), files[0].Data, "test.json")
	})

	t.Run("generating openAPI from cue map of any", func(t *testing.T) {
		req := require.New(t)

		mapAnySchema := `
MapOfStringToAny: [string]: _
`

		expectedMapAnyOpenAPI := `{
  "openapi": "3.0.0",
  "info": {
    "title": "sandbox",
    "version": "0.0.0",
    "x-schema-identifier": "",
    "x-schema-kind": ""
  },
  "paths": {},
  "components": {
    "schemas": {
      "MapOfStringToAny": {
        "type": "object",
        "additionalProperties": true
      }
    }
  }
}`

		cueValue := cuecontext.New().CompileString(mapAnySchema)
		req.NoError(cueValue.Err())

		files, err := TypesFromSchema().
			CUEValue("sandbox", cueValue).
			GenerateOpenAPI(OpenAPIGenerationConfig{}).
			Run(context.Background())
		req.NoError(err)

		req.Len(files, 1, "expected a single file")
		testutils.TrimSpacesDiffComparator(t, []byte(expectedMapAnyOpenAPI), files[0].Data, "test.json")
	})
}

func TestTypesFromSchema_SchemaTransformations(t *testing.T) {
	schema := `
// Contains things.
Container: {
    str: string
}
`

	expectedCode := `// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package sandbox

// Contains things.
// Transformed by cog.
type ExampleContainer struct {
	Str string ` + "`json:\"str\"`" + `
}

// NewExampleContainer creates a new ExampleContainer object.
func NewExampleContainer() *ExampleContainer {
	return &ExampleContainer{}
}`

	t.Run("transformations can be applied on the input schema", func(t *testing.T) {
		req := require.New(t)

		cueValue := cuecontext.New().CompileString(schema)
		req.NoError(cueValue.Err())

		files, err := TypesFromSchema().
			CUEValue("sandbox", cueValue).
			Golang(GoConfig{}).
			SchemaTransformations(
				AppendCommentToObjects("Transformed by cog."),
				PrefixObjectsNames("Example"),
			).
			Run(context.Background())
		req.NoError(err)

		req.Len(files, 1, "expected a single file")
		testutils.TrimSpacesDiffComparator(t, []byte(expectedCode), files[0].Data, "test.go")
	})
}
