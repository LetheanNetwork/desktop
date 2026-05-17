# HIBP / NCSC Top-100K Passwords — Attribution

The embedded SHA-1 digest set at
`go/pkg/passphrase/data/top100k.bin` is derived from publicly
published breach corpora. This document tracks attribution,
licence, and the recipe to regenerate or bump the dataset.

## Source

Plaintext source list:
`100k-most-used-passwords-NCSC.txt` from the SecLists project
— a deduplicated, frequency-sorted top 100,000 leaked
passwords list, compiled by the UK National Cyber Security
Centre (NCSC) from the Have I Been Pwned (HIBP) Pwned
Passwords corpus.

- SecLists path:
  https://github.com/danielmiessler/SecLists/blob/master/Passwords/Common-Credentials/100k-most-used-passwords-NCSC.txt
- HIBP project: https://haveibeenpwned.com/Passwords
- NCSC reference:
  https://www.ncsc.gov.uk/blog-post/passwords-passwords-everywhere

Source file size: ~836 KB plaintext, 99,840 lines (one
duplicate after normalisation), 99,839 unique SHA-1 digests
embedded.

## Licences

| Layer | Licence | Notes |
|-------|---------|-------|
| Have I Been Pwned (Pwned Passwords corpus) | Creative Commons Attribution 4.0 (CC-BY-4.0) | https://haveibeenpwned.com/API/v3 — "all data is licenced under CC BY 4.0" |
| NCSC published "top 100k" list | UK Open Government Licence v3.0 (OGL-UK-3.0) | https://www.nationalarchives.gov.uk/doc/open-government-licence/version/3/ |
| SecLists project (aggregator) | MIT | https://github.com/danielmiessler/SecLists/blob/master/LICENSE |

All three are compatible with EUPL-1.2 (this project's
licence). We ship only the SHA-1 digests, not the original
plaintexts — derivative work, attribution preserved here.

## What we store

`data/top100k.bin` is a concatenation of 99,839 unique 20-byte
SHA-1 digests, lexicographically sorted on disk. Total size
~1.9 MB. No plaintext is retained. A binary inspection (`file`,
`strings`, `hexdump`) yields opaque entropy — no readable
dictionary leaks to anyone reading our shipped artefacts.

## Regeneration recipe

When the upstream NCSC / HIBP list refreshes (HIBP v9, etc), or
when Snider routes a switch to a different breach corpus, the
binary blob is regenerated from the same generator that built
the v1 ship:

```go
// gen.go — run from a scratch directory containing top100k.txt
package main

import (
    "bufio"
    "bytes"
    "crypto/sha1"
    "fmt"
    "os"
    "sort"
)

func main() {
    f, _ := os.Open("top100k.txt")
    defer f.Close()
    seen := make(map[[20]byte]struct{}, 110000)
    s := bufio.NewScanner(f)
    s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
    for s.Scan() {
        line := s.Text()
        if line == "" {
            continue
        }
        sum := sha1.Sum([]byte(line))
        seen[sum] = struct{}{}
    }
    all := make([][20]byte, 0, len(seen))
    for h := range seen {
        all = append(all, h)
    }
    sort.Slice(all, func(i, j int) bool {
        return bytes.Compare(all[i][:], all[j][:]) < 0
    })
    out, _ := os.Create("top100k.bin")
    defer out.Close()
    for _, h := range all {
        out.Write(h[:])
    }
    fmt.Printf("wrote %d digests (%d bytes)\n", len(all), len(all)*20)
}
```

Then replace `go/pkg/passphrase/data/top100k.bin` with the new
file and run `go test ./go/pkg/passphrase/...`. No Go-code
changes required — the loader treats the embed length as the
source of truth.

## Why this exists in the repo

Stage X RFC v2 §5.1 HIGH-3 requires the passphrase rejection
gate to consult a top-N HIBP-style leaked-passwords list, not
the v1 ~50-entry seed that shipped earlier. Mantis #1507 is
the dataset bump. This file records the attribution chain that
the HIBP / NCSC / SecLists licences require.

— Hephaestus, 2026-05-17
