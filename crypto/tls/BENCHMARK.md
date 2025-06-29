# Benchmark Results for SNI Package

**Command run:**

```bash
go test -bench=. -benchtime 100000000x -benchmem
````

**Test data**

* `testdata/sni` contains byte data generated using `nc` and `curl`:

```bash
nc -l 8443 > "$server" &
curl -v --resolve $"$server:8443:127.0.0.1" $"https://$server:8443/" --max-time 1
```

---

## System Information

* **OS:** linux
* **Architecture:** amd64
* **CPU:** Intel(R) Core(TM) i5-6500 CPU @ 3.20GHz
* **Package:** github.com/FMotalleb/junction/crypto/sni

---

## Benchmark Output

| Function                      | Time/op   | Throughput       | Allocations | Bytes/op |
|-------------------------------|-----------|------------------|-------------|----------|
| ExtractSNI                    | 19.12 ns  | 82.10 GB/s       | 0           | 0 B      |
| UnmarshalClientHello          | 145.3 ns  | 10.81 GB/s       | 0           | 0 B      |

---

## Summary

* `ExtractSNI` is an ultra-fast, zero-alloc method for extracting only the SNI from TLS handshakes.
  * ~6-8x faster than **ClientHello.Unmarshal** as it skips every thing to find first hostname.
  * Hello Packet supports more than one SNI by standards but in real world there is only one SNI in this packet.
  * Best option if only the SNI is needed.
* `ClientHello.Unmarshal` provides full TLS ClientHello parsing at a reasonable computation cost.
