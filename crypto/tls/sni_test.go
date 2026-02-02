package tls_test

import (
	"embed"
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/fmotalleb/junction/crypto/tls"
)

//go:embed testdata/sni/*
var testFiles embed.FS

const (
	datadir     = "testdata/sni"
	benchmarkOn = "google.com"
)

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
