// SPDX-Licence-Identifier: EUPL-1.2

// dataset.go — embeds the top-100K leaked-passphrase digest set
// for IsCommon's lookup table. Bumps the v1 ~50-entry seed (kept
// below as a documented audit-transparency sample) to ~99,839
// unique SHA-1 digests drawn from NCSC's "100K Most Used
// Passwords" list, itself derived from the HIBP Pwned Passwords
// corpus.
//
// Source file: data/top100k.bin — packed binary of 20-byte SHA-1
// digests, lexicographically sorted on disk so the runtime
// loader can mmap-style slice the embed without re-sorting.
// Sourced from SecLists' 100k-most-used-passwords-NCSC.txt
// (https://github.com/danielmiessler/SecLists/blob/master/Passwords/Common-Credentials/100k-most-used-passwords-NCSC.txt)
// — the NCSC's HIBP-derived top-100K list. Attribution +
// licence text at /LICENSES/HIBP.md.
//
// Why hashes not plaintexts: a binary inspection (file, strings,
// hex-dump) doesn't surface a readable leaked-password
// dictionary. The 1.9MB binary is opaque entropy by inspection.
//
// Generation: see /LICENSES/HIBP.md for the one-shot generator
// recipe. To bump the dataset (HIBP v9 release, additional
// breach corpora, etc), re-run the generator against the new
// source + replace data/top100k.bin. The Go code does not
// change.
//
// Why a sample stays in source: the auditSample slice below
// reproduces ~48 well-known leaked plaintexts with their SHA-1
// digests. It's NOT loaded at runtime (the embed is the source
// of truth) — it exists so a human reading the Go code can see
// "yes, 'password' is in the dataset and its hash is X" without
// having to disassemble the binary. Each sample is verified
// against the embedded set in TestPassphrase_AuditSample_Good
// so the comments cannot lie.

package passphrase

import _ "embed"

// top100kBin holds the embedded SHA-1 digest set. Format:
// concatenated 20-byte digests, lexicographically sorted. Load
// + slice happens in passphrase.go's init().
//
//go:embed data/top100k.bin
var top100kBin []byte

// auditSample lists a curated subset of well-known leaked
// plaintexts paired with their SHA-1 hex digests. NOT loaded
// at runtime — solely for human auditability of the embedded
// dataset. Verified against top100kBin by
// TestPassphrase_AuditSample_Good so any drift fires a test.
//
// Roughly frequency-sorted (most-common first). Plaintext column
// is the audit transparency: every entry below is in the
// embedded dataset because the embedded dataset includes the
// NCSC top-100K and these are all in the top-100K.
var auditSample = []struct {
	Plaintext string
	SHA1Hex   string
}{
	{"password", "5baa61e4c9b93f3f0682250b6cf8331b7ee68fd8"},
	{"123456", "7c4a8d09ca3762af61e59520943dc26494f8941b"},
	{"12345678", "7c222fb2927d828af22f592134e8932480637c0d"},
	{"qwerty", "b1b3773a05c0ed0176787a4f1574ff0075f7521e"},
	{"abc123", "6367c48dd193d56ea7b0baad25b19455e529f5ee"},
	{"monkey", "ab87d24bdc7452e55738deb5f868e1f16dea5ace"},
	{"letmein", "b7a875fc1ea228b9061041b7cec4bd3c52ab3ce3"},
	{"trustno1", "e68e11be8b70e435c65aef8ba9798ff7775c361e"},
	{"dragon", "af8978b1797b72acfff9595a5a2a373ec3d9106d"},
	{"baseball", "a2c901c8c6dea98958c219f6f2d038c44dc5d362"},
	{"111111", "3d4f2bf07dc1be38b20cd6e46949a1071f9d0e3d"},
	{"iloveyou", "ee8d8728f435fd550f83852aabab5234ce1da528"},
	{"master", "4f26aeafdb2367620a393c973eddbe8f8b846ebd"},
	{"sunshine", "8d6e34f987851aa599257d3831a1af040886842f"},
	{"ashley", "782f9b10621e362d5bd0def3a279b5e0908c9ebb"},
	{"bailey", "b1f45ed147d6803ac1a2a91bdea1fab603f910a5"},
	{"passw0rd", "7c6a61c68ef8b9b6b061b28c348bc1ed7921cb53"},
	{"shadow", "ed9d3d832af899035363a69fd53cd3be8f71501c"},
	{"123123", "601f1889667efaebb33b8c12572835da3f027f78"},
	{"654321", "dd5fef9c1c1da1394d6d34b248c51be2ad740840"},
	{"superman", "18c28604dd31094a8d69dae60f1bcd347f1afc5a"},
	{"qazwsx", "cb45c671cbc500627ea424eea5f91996221b5935"},
	{"michael", "17b9e1c64588c7fa6419b4d29dc1f4426279ba01"},
	{"football", "2d27b62c597ec858f6e7b54e7e58525e6a95e6d8"},
	{"welcome", "c0b137fe2d792459f26ff763cce44574a5b5ab03"},
	{"admin", "d033e22ae348aeb5660fc2140aec35850c4da997"},
	{"admin123", "f865b53623b121fd34ee5426c792e5c33af8c227"},
	{"password1", "e38ad214943daad1d64c102faec29de4afe9da3d"},
	{"password123", "cbfdac6008f9cab4083784cbd1874f76618d2a97"},
	{"passwordpassword", "476e251cc54b60534f68d0f614fcc67950151353"},
	{"qwerty123", "5cec175b165e3d5e62c9e13ce848ef6feac81bff"},
	{"qwertyuiop", "b0399d2029f64d445bd131ffaa399a42d2f8e7dc"},
	{"1q2w3e4r", "48efc4851e15940af5d477d3c0ce99211a70a3be"},
	{"1qaz2wsx", "c6922b6ba9e0939583f973bc1682493351ad4fe8"},
	{"zaq12wsx", "cdf547ed4c64e6994af35cfcd69c4204c9227a97"},
	{"123321", "4d9012b4a77a9524d675dad27c3276ab5705e5e8"},
	{"666666", "1411678a0b9e25ee2f7c8b2f7ac92b6a74b3f9c5"},
	{"princess", "775bb961b81da1ca49217a48e533c832c337154a"},
	{"azerty", "9cf95dacd226dcf43da376cdb6cbba7035218921"},
	{"batman", "5c6d9edc3a951cda763f650235cfc41a3fc23fe8"},
	{"asdf", "3da541559918a808c2402bba5012f6c60b27661c"},
	{"hello", "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"},
	{"freedom", "7ecfd8f97b4729c6ff0799b0b4d40f870083b461"},
	{"whatever", "d869db7fe62fb07c25a0403ecaea55031744b5fb"},
	{"qwerty1", "ad70ab97ae1376e656002641cfb067c9c94906a2"},
	{"letmein1", "d04c1675b232c6ece69ed95e189e95d589f217b0"},
}
