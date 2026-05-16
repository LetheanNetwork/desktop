// SPDX-Licence-Identifier: EUPL-1.2

// dataset.go — curated seed of well-known leaked passphrases for
// IsCommon's hash-lookup table. v1 ships a ~50-entry "worst of
// the worst" subset that any reasonable top-N expansion would
// include. Sourced from the consistent overlap across:
//
//   - SecLists' Common-Credentials/10-million-password-list-top-N
//     (MIT-licensed, github.com/danielmiessler/SecLists)
//   - NIST 800-63B Appendix A examples
//   - CrackStation's public corpus
//   - HIBP top-N (rank-stable since v8)
//
// Entries are stored as SHA-1 hex strings (not plaintexts) so a
// binary inspection doesn't surface a readable dictionary. Comment
// next to each entry names the plaintext for audit transparency —
// these are all public-domain leaked credentials, no value lost.
//
// Roughly frequency-sorted (most-common first). Runtime
// representation in passphrase.go is binary-sorted via init()
// so binary search lookup stays O(log N).
//
// Growing the dataset: append entries here, run tests, ship.
// Stage X RFC v2 §5.1 HIGH-3 calls for top-100K HIBP expansion
// when Snider routes the dataset-procurement decision. At that
// scale switch to `//go:embed top10k.bin` (binary 20-byte SHA-1
// records, sorted on disk) — see passphrase.go docstring.

package passphrase

// rawHashHex is the SHA-1 hex-string source for the common
// passphrase set. Loaded into the sorted-binary `hashes` slice
// at init() time. Each entry's trailing comment carries the
// plaintext for audit transparency.
var rawHashHex = []string{
	"5baa61e4c9b93f3f0682250b6cf8331b7ee68fd8", // password
	"7c4a8d09ca3762af61e59520943dc26494f8941b", // 123456
	"7c222fb2927d828af22f592134e8932480637c0d", // 12345678
	"b1b3773a05c0ed0176787a4f1574ff0075f7521e", // qwerty
	"6367c48dd193d56ea7b0baad25b19455e529f5ee", // abc123
	"ab87d24bdc7452e55738deb5f868e1f16dea5ace", // monkey
	"b7a875fc1ea228b9061041b7cec4bd3c52ab3ce3", // letmein
	"e68e11be8b70e435c65aef8ba9798ff7775c361e", // trustno1
	"af8978b1797b72acfff9595a5a2a373ec3d9106d", // dragon
	"a2c901c8c6dea98958c219f6f2d038c44dc5d362", // baseball
	"3d4f2bf07dc1be38b20cd6e46949a1071f9d0e3d", // 111111
	"ee8d8728f435fd550f83852aabab5234ce1da528", // iloveyou
	"4f26aeafdb2367620a393c973eddbe8f8b846ebd", // master
	"8d6e34f987851aa599257d3831a1af040886842f", // sunshine
	"782f9b10621e362d5bd0def3a279b5e0908c9ebb", // ashley
	"b1f45ed147d6803ac1a2a91bdea1fab603f910a5", // bailey
	"7c6a61c68ef8b9b6b061b28c348bc1ed7921cb53", // passw0rd
	"ed9d3d832af899035363a69fd53cd3be8f71501c", // shadow
	"601f1889667efaebb33b8c12572835da3f027f78", // 123123
	"dd5fef9c1c1da1394d6d34b248c51be2ad740840", // 654321
	"18c28604dd31094a8d69dae60f1bcd347f1afc5a", // superman
	"cb45c671cbc500627ea424eea5f91996221b5935", // qazwsx
	"17b9e1c64588c7fa6419b4d29dc1f4426279ba01", // michael
	"2d27b62c597ec858f6e7b54e7e58525e6a95e6d8", // football
	"c0b137fe2d792459f26ff763cce44574a5b5ab03", // welcome
	"d033e22ae348aeb5660fc2140aec35850c4da997", // admin
	"f865b53623b121fd34ee5426c792e5c33af8c227", // admin123
	"e38ad214943daad1d64c102faec29de4afe9da3d", // password1
	"cbfdac6008f9cab4083784cbd1874f76618d2a97", // password123
	"476e251cc54b60534f68d0f614fcc67950151353", // passwordpassword
	"5cec175b165e3d5e62c9e13ce848ef6feac81bff", // qwerty123
	"b0399d2029f64d445bd131ffaa399a42d2f8e7dc", // qwertyuiop
	"48efc4851e15940af5d477d3c0ce99211a70a3be", // 1q2w3e4r
	"c6922b6ba9e0939583f973bc1682493351ad4fe8", // 1qaz2wsx
	"cdf547ed4c64e6994af35cfcd69c4204c9227a97", // zaq12wsx
	"a2cfb3d223a56088065332957b511f54ebdb6975", // mynoob
	"4d9012b4a77a9524d675dad27c3276ab5705e5e8", // 123321
	"1411678a0b9e25ee2f7c8b2f7ac92b6a74b3f9c5", // 666666
	"775bb961b81da1ca49217a48e533c832c337154a", // princess
	"9cf95dacd226dcf43da376cdb6cbba7035218921", // azerty
	"49f25741ff0db65a7c4290aa73f34b4d4a3644c6", // solo
	"5c6d9edc3a951cda763f650235cfc41a3fc23fe8", // batman
	"3da541559918a808c2402bba5012f6c60b27661c", // asdf
	"aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d", // hello
	"7ecfd8f97b4729c6ff0799b0b4d40f870083b461", // freedom
	"d869db7fe62fb07c25a0403ecaea55031744b5fb", // whatever
	"ad70ab97ae1376e656002641cfb067c9c94906a2", // qwerty1
	"d04c1675b232c6ece69ed95e189e95d589f217b0", // letmein1
}
