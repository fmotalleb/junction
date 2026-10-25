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

func TestExtractSNIRejectsMalformedAndAcceptsFragmentedRecords(t *testing.T) {
	malformed := make([]byte, 43)
	malformed[0], malformed[5] = 0x16, 0x01
	if got := tls.ExtractSNI(malformed); got != nil {
		t.Fatalf("malformed ClientHello unexpectedly returned SNI %q", got)
	}

	hello := testData["google.com"]
	recordLen := int(hello[3])<<8 | int(hello[4])
	firstPayload := recordLen / 2
	first := append([]byte(nil), hello[:5+firstPayload]...)
	first[3] = byte(firstPayload >> 8)
	first[4] = byte(firstPayload)
	secondPayload := hello[5+firstPayload : 5+recordLen]
	second := []byte{0x16, hello[1], hello[2], byte(len(secondPayload) >> 8), byte(len(secondPayload))}
	second = append(second, secondPayload...)
	fragmented := append(first, second...)
	if got := string(tls.ExtractSNI(fragmented)); got != "google.com" {
		t.Fatalf("fragmented ClientHello SNI = %q, want google.com", got)
	}
}

func FuzzExtractSNI(f *testing.F) {
	f.Add([]byte{0x16, 0x03, 0x03, 0, 38, 1, 0, 0, 34})
	f.Add(testData["google.com"])
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = tls.ExtractSNI(data)
		var hello tls.ClientHello
		_ = hello.Unmarshal(data)
	})
}

func TestUnmarshalClientHello(t *testing.T) {
	for domain, data := range testData {
		info := new(tls.ClientHello)
		err := info.Unmarshal(data)
		assert.NoError(t, err, "failed to parse")
		assert.Equal(t, domain, string(info.SNIHostNames[0]))
	}
}

func TestSNIParsersAgreeOnSingleRecordCorpus(t *testing.T) {
	valid := testData["google.com"]
	malformed := make([]byte, 43)
	malformed[0], malformed[5] = 0x16, 0x01
	badRecordType := append([]byte(nil), valid...)
	badRecordType[0] = 0x17
	badHandshakeType := append([]byte(nil), valid...)
	badHandshakeType[5] = 0x02
	badHandshakeLength := append([]byte(nil), valid...)
	badHandshakeLength[6], badHandshakeLength[7], badHandshakeLength[8] = 0xff, 0xff, 0xff
	badExtensionsLength := append([]byte(nil), valid...)
	badExtensionsLength[len(badExtensionsLength)-2] = 0xff

	corpus := map[string][]byte{
		"valid":                 valid,
		"panic regression":      malformed,
		"bad record type":       badRecordType,
		"bad handshake type":    badHandshakeType,
		"bad handshake length":  badHandshakeLength,
		"bad extensions length": badExtensionsLength,
	}
	for name, input := range corpus {
		t.Run(name, func(t *testing.T) {
			gotExtract := tls.ExtractSNI(input)
			var parsed tls.ClientHello
			var gotUnmarshal []byte
			if err := parsed.Unmarshal(input); err == nil && parsed.SNICount > 0 {
				gotUnmarshal = parsed.SNIHostNames[0]
			}
			assert.Equal(t, string(gotExtract), string(gotUnmarshal))
		})
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
