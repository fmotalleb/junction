package tls_test

import (
	"embed"
	"fmt"
	"os"
	"testing"

	"github.com/FMotalleb/junction/crypto/tls"
	"github.com/alecthomas/assert/v2"
)

//go:embed testdata/sni/*
var testFiles embed.FS

const (
	datadir     = "testdata/sni"
	benchmarkOn = "google.com"
)

var testData = make(map[string][]byte)

func wrapErr(msg string, err error) error {
	return fmt.Errorf("%s: %w", msg, err)
}

func TestMain(m *testing.M) {
	var err error
	files, err := testFiles.ReadDir(datadir)
	if err != nil {
		panic(wrapErr("failed to read embed fs", err))
	}

	for _, f := range files {
		if !f.IsDir() {
			name := f.Name()
			path := datadir + "/" + name
			testData[name], err = testFiles.ReadFile(path)
			if err != nil {
				panic(wrapErr("failed to populate test data", err))
			}
		}
	}

	os.Exit(m.Run())
}

func TestExtractSNI(t *testing.T) {
	for domain, data := range testData {
		info := tls.ExtractSNI(data)
		assert.Equal(t, domain, string(info))
	}
}

func TestUnmarshalClientHello(t *testing.T) {
	for domain, data := range testData {
		info := new(tls.ClientHello)
		err := info.Unmarshal(data)
		assert.NoError(t, err, "failed to parse")
		assert.Equal(t, domain, string(info.SNIHostNames[0]))
	}
}

func BenchmarkExtractSNI(b *testing.B) {
	data := testData[benchmarkOn]
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tls.ExtractSNI(data)
	}
}

func BenchmarkUnmarshalClientHello(b *testing.B) {
	data := testData[benchmarkOn]
	b.SetBytes(int64(len(data)))
	info := new(tls.ClientHello)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if info.Unmarshal(data) != nil {
			b.Fail()
		}
	}
}
