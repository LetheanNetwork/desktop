// SPDX-Licence-Identifier: EUPL-1.2

package terminal

import "fmt"

func ExampleTerminalSession_SubscribeFrom() {
	session := memSession(8)
	session.appendRing([]byte("hello"))

	replay, unsubscribe, err := session.SubscribeFrom(2, func(chunk OutputChunk) {
		fmt.Printf("live %d..%d %q\n", chunk.Start, chunk.End, chunk.Data)
	})
	if err != nil {
		panic(err)
	}
	defer unsubscribe()

	fmt.Printf("replay %d..%d %q\n", replay.Start, replay.End, replay.Data)
	session.publish([]byte("!"))

	// Output:
	// replay 2..5 "llo"
	// live 5..6 "!"
}
