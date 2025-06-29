package sni_test

import (
	"embed"
	"os"
	"path/filepath"
	"testing"

	"github.com/FMotalleb/junction/crypto/sni"
	"github.com/alecthomas/assert/v2"
)

//go:embed testdata/*
var testFiles embed.FS

const datadir = "testdata"

var testData = make(map[string][]byte)

func TestMain(m *testing.M) {
	var err error
	files, err := testFiles.ReadDir(datadir)
	if err != nil {
		panic(err)
	}

	for _, f := range files {
		if !f.IsDir() {
			name := f.Name()
			path := filepath.Join(datadir, name)
			testData[name], err = testFiles.ReadFile(path)
			if err != nil {
				panic(err)
			}
		}
	}

	os.Exit(m.Run())
}

func TestExtractHost(t *testing.T) {
	for domain, data := range testData {
		info := sni.ExtractHost(data)
		assert.Equal(t, domain, string(info))
	}
}

func TestParseClientHello(t *testing.T) {
	for domain, data := range testData {
		info, err := sni.ParseClientHello(data)
		assert.NoError(t, err, "failed to parse")
		assert.Equal(t, domain, info.SNIHostNames[0])
	}
}

func BenchmarkExtractHost(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, data := range testData {
			sni.ExtractHost(data)
			// if parsed != domain {
			// 	assert.Equal(b, domain, parsed)
			// }
		}
	}
}

func BenchmarkParseClientHello(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for domain, data := range testData {
			info, err := sni.ParseClientHello(data)
			if err != nil {
				assert.NoError(b, err, "failed to parse hello")
			}
			if info.SNIHostNames[0] != domain {
				assert.Equal(b, domain, info.SNIHostNames[0])
			}
		}
	}
}
