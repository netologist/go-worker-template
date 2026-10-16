package arch_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

const modulePrefix = "github.com/netologist/go-worker-template"

func getProjectRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)

	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "could not find go.mod in parent directories")
		dir = parent
	}
}

// loadInternalPackages loads production packages under internal/ excluding generated code.
func loadInternalPackages(t *testing.T) map[string][]string {
	t.Helper()
	root := getProjectRoot(t)

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedImports,
		Dir:  root,
	}

	pkgs, err := packages.Load(cfg, "./internal/...")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	result := make(map[string][]string)
	for _, pkg := range pkgs {
		if strings.Contains(pkg.PkgPath, "/internal/gen") {
			continue
		}
		var imps []string
		for _, imp := range pkg.Imports {
			imps = append(imps, imp.PkgPath)
		}
		result[pkg.PkgPath] = imps
	}

	return result
}

func TestArchitecture_DomainLayerPurity(t *testing.T) {
	t.Parallel()
	pkgs := loadInternalPackages(t)

	domainPath := modulePrefix + "/internal/domain"
	domainImports, ok := pkgs[domainPath]
	require.True(t, ok, "internal/domain must exist in loaded packages")

	allowedExternalPrefixes := []string{
		"github.com/google/uuid",
	}

	for _, imp := range domainImports {
		// Domain must never import outer internal layers
		assert.False(
			t,
			strings.HasPrefix(imp, modulePrefix+"/internal/usecase"),
			"internal/domain must not import usecase: %s", imp,
		)
		assert.False(
			t,
			strings.HasPrefix(imp, modulePrefix+"/internal/adapter"),
			"internal/domain must not import adapters: %s", imp,
		)
		assert.False(
			t,
			strings.HasPrefix(imp, modulePrefix+"/internal/config"),
			"internal/domain must not import config: %s", imp,
		)
		assert.False(
			t,
			strings.HasPrefix(imp, modulePrefix+"/internal/gen"),
			"internal/domain must not import generated code: %s", imp,
		)

		// Domain must never import transport, broker SDKs, or database drivers
		assert.False(
			t,
			strings.HasPrefix(imp, "net/http") || strings.Contains(imp, "amqp") || strings.Contains(imp, "kafka"),
			"internal/domain must not import transport or broker SDKs: %s", imp,
		)
		assert.False(
			t,
			strings.HasPrefix(imp, "database/sql") || strings.Contains(imp, "pgx"),
			"internal/domain must not import database drivers: %s", imp,
		)

		// Third-party packages check
		if strings.Contains(imp, ".") {
			isAllowed := false
			for _, allowed := range allowedExternalPrefixes {
				if strings.HasPrefix(imp, allowed) {
					isAllowed = true
					break
				}
			}
			assert.True(
				t,
				isAllowed,
				"internal/domain can only import standard library or allowed primitives (uuid): %s", imp,
			)
		}
	}
}

func TestArchitecture_UsecaseLayerIsolation(t *testing.T) {
	t.Parallel()
	pkgs := loadInternalPackages(t)

	usecasePath := modulePrefix + "/internal/usecase"
	usecaseImports, ok := pkgs[usecasePath]
	require.True(t, ok, "internal/usecase must exist in loaded packages")

	for _, imp := range usecaseImports {
		// Usecases must never import outer adapters or config
		assert.False(
			t,
			strings.HasPrefix(imp, modulePrefix+"/internal/adapter"),
			"internal/usecase must not import outer adapters: %s", imp,
		)
		assert.False(
			t,
			strings.HasPrefix(imp, modulePrefix+"/internal/config"),
			"internal/usecase must not import config: %s", imp,
		)
		assert.False(
			t,
			strings.HasPrefix(imp, modulePrefix+"/cmd"),
			"internal/usecase must not import cmd: %s", imp,
		)

		// Usecases must never import broker SDKs or database drivers directly
		assert.False(
			t,
			strings.Contains(imp, "amqp") || strings.Contains(imp, "kafka") || strings.Contains(imp, "pgx"),
			"internal/usecase must not import broker or database drivers: %s", imp,
		)
	}
}

func TestArchitecture_AdapterIsolation(t *testing.T) {
	t.Parallel()
	pkgs := loadInternalPackages(t)

	postgresPath := modulePrefix + "/internal/adapter/postgres"
	postgresImports, ok := pkgs[postgresPath]
	require.True(t, ok, "internal/adapter/postgres must exist in loaded packages")

	for _, imp := range postgresImports {
		assert.False(
			t,
			strings.HasPrefix(imp, modulePrefix+"/internal/adapter/consumer"),
			"postgres adapter must not cross-import consumer adapter: %s", imp,
		)
		assert.False(
			t,
			strings.HasPrefix(imp, modulePrefix+"/internal/adapter/probe"),
			"postgres adapter must not cross-import probe adapter: %s", imp,
		)
	}

	probePath := modulePrefix + "/internal/adapter/probe"
	probeImports, ok := pkgs[probePath]
	require.True(t, ok, "internal/adapter/probe must exist in loaded packages")

	for _, imp := range probeImports {
		assert.False(
			t,
			strings.HasPrefix(imp, modulePrefix+"/internal/adapter/postgres"),
			"probe adapter must not cross-import postgres adapter: %s", imp,
		)
		assert.False(
			t,
			strings.HasPrefix(imp, modulePrefix+"/internal/adapter/consumer"),
			"probe adapter must not cross-import consumer adapter: %s", imp,
		)
	}
}

func TestArchitecture_ConfigLayerIsolation(t *testing.T) {
	t.Parallel()
	pkgs := loadInternalPackages(t)

	configPath := modulePrefix + "/internal/config"
	configImports, ok := pkgs[configPath]
	require.True(t, ok, "internal/config must exist in loaded packages")

	for _, imp := range configImports {
		assert.False(
			t,
			strings.HasPrefix(imp, modulePrefix+"/internal/domain"),
			"internal/config must not import domain: %s", imp,
		)
		assert.False(
			t,
			strings.HasPrefix(imp, modulePrefix+"/internal/usecase"),
			"internal/config must not import usecase: %s", imp,
		)
		assert.False(
			t,
			strings.HasPrefix(imp, modulePrefix+"/internal/adapter"),
			"internal/config must not import adapter: %s", imp,
		)
	}
}
