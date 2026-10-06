package config

import (
	"os"
	"reflect"
	"testing"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const exampleFile = "../../config.example.yaml"

// TestExampleFile_IsValid keeps config.example.yaml loadable.
func TestExampleFile_IsValid(t *testing.T) {
	_, err := Load(Source{File: exampleFile})
	require.NoError(t, err)
}

// TestExampleFile_DocumentsEverySetting fails when a setting is added to Config
// without being documented in config.example.yaml.
func TestExampleFile_DocumentsEverySetting(t *testing.T) {
	data, err := os.ReadFile(exampleFile)
	require.NoError(t, err)
	k := koanf.New(keyDelimiter)
	require.NoError(t, k.Load(rawbytes.Provider(data), yaml.Parser()))

	for key := range schemaOf(reflect.TypeFor[Config]()).leaves {
		assert.True(t, k.Exists(key), "config.example.yaml does not document %q", key)
	}
}
