package main

import (
	"fmt"

	"github.com/nats-io/nats.go"
)

func natsTest() {

	url := "nats://43.160.212.55:4222"
	// Connect to a server
	nc, _ := nats.Connect(url)

	// Simple Async Subscriber
	nc.Subscribe("foo", func(m *nats.Msg) {
		fmt.Printf("Received a message: %s\n", string(m.Data))
	})

	nc.Flush()

	// Simple Publisher
	nc.Publish("foo", []byte("Hello World"))

	nc.Flush()

	//// Responding to a request message
	//nc.Subscribe("request", func(m *nats.Msg) {
	//	m.Respond([]byte("answer is 42"))
	//})
	//
	//// Simple Sync Subscriber
	//sub, _ := nc.SubscribeSync("foo")
	//timeOut := time.Duration(5 * time.Second)
	//
	//m, _ := sub.NextMsg(timeOut)
	//
	//fmt.Printf("Received a message: %s\n", string(m.Data))
	//
	//// Channel Subscriber
	//ch := make(chan *nats.Msg, 64)
	//sub1, _ := nc.ChanSubscribe("foo", ch)
	//
	//fmt.Printf("Subscribed to %s\n", sub1.Subject)
	//
	//msg1 := <-ch
	//
	//fmt.Printf("Received a message: %s\n", string(msg1.Data))
	//// Unsubscribe
	//sub.Unsubscribe()
	//
	//// Drain
	//sub.Drain()
	//
	//// Requests
	//msg2, _ := nc.Request("help", []byte("help me"), 10*time.Millisecond)
	//
	//fmt.Printf("Received a message: %s\n", string(msg2.Data))
	//
	//// Replies
	//nc.Subscribe("help", func(m *nats.Msg) {
	//	nc.Publish(m.Reply, []byte("I can help!"))
	//})

	// Drain connection (Preferred for responders)
	// Close() not needed if this is called.
	nc.Drain()

	// Close connection
	nc.Close()
}
func main() {
	jsTest()
}
