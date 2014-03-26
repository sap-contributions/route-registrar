package registrar_test

import (
	"os"
	"syscall"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/cloudfoundry/yagnats"

	. "github.com/cloudfoundry-incubator/route-registrar/config"
	. "github.com/cloudfoundry-incubator/route-registrar/registrar"
)

var config Config
var client *yagnats.Client

var _ = Describe("Src/Main/RouteRegister", func() {
	messageBusServer := MessageBusServer{
		"127.0.0.1:4222",
		"nats",
		"nats",
	}

	config = Config{
		messageBusServer,
		"riakcs.vcap.me",
		"127.0.0.1",
		8080,
	}

	BeforeEach(func(){
		client = yagnats.NewClient()
		connectionInfo := yagnats.ConnectionInfo{
			config.MessageBusServer.Host,
			config.MessageBusServer.User,
			config.MessageBusServer.Password,
		}

		err := client.Connect(&connectionInfo)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func(){
		client.Disconnect()
	})

	It("Sends a router.register message and does not send a router.unregister message", func() {
		// Detect when a router.register message gets sent
		var registered chan(string)
		registered = subscribeToRegisterEvents(func(msg *yagnats.Message) {
			fmt.Println("GOT REGISTER MESSAGE: ", string(msg.Payload))
			registered <- string(msg.Payload)
		})

		// Detect when an unregister message gets sent
		var unregistered chan(bool)
		unregistered = subscribeToUnregisterEvents(func(msg *yagnats.Message) {
			fmt.Println("GOT UNREGISTER MESSAGE: ", string(msg.Payload))
			unregistered <- true
		})

		go func () {
			registrar := NewRegistrar(config)
			fmt.Println("Registering routes...")
			registrar.RegisterRoutes()
		}()

		// Assert that we got the right router.register message
		var receivedMessage string
		Eventually(registered, 2).Should(Receive(&receivedMessage))
		Expect(receivedMessage).To(Equal(`{"uris":["riakcs.vcap.me"],"host":"127.0.0.1","port":8080}`))

		// Assert that we never got a router.unregister message
		Consistently(unregistered, 2).ShouldNot(Receive())
	})


	It("Emits a router.unregister message when SIGINT is sent to the registrar's signal channel", func () {
		verifySignalTriggersUnregister(syscall.SIGINT)
	})

	It("Emits a router.unregister message when SIGTERM is sent to the registrar's signal channel", func () {
		verifySignalTriggersUnregister(syscall.SIGTERM)
	})
})

func verifySignalTriggersUnregister(signal os.Signal){
	unregistered := make(chan string)
	returned := make(chan bool)

	var registrar *Registrar

	// Trigger a SIGINT after a successful router.register message
	subscribeToRegisterEvents(func(msg *yagnats.Message) {
		fmt.Println("GOT REGISTER MESSAGE: ", string(msg.Payload))
		registrar.SignalChannel <- signal
	})

	// Detect when a router.unregister message gets sent
	subscribeToUnregisterEvents(func(msg *yagnats.Message) {
		fmt.Println("GOT UNREGISTER MESSAGE: ", string(msg.Payload))
		unregistered <- string(msg.Payload)
	})

	go func () {
		registrar = NewRegistrar(config)
		fmt.Println("Registering routes...")
		registrar.RegisterRoutes()

		// Set up a channel to wait for RegisterRoutes to return
		returned <- true
	}()

	// Assert that we got the right router.unregister message as a result of the signal
	var receivedMessage string
	Eventually(unregistered, 2).Should(Receive(&receivedMessage))
	Expect(receivedMessage).To(Equal(`{"uris":["riakcs.vcap.me"],"host":"127.0.0.1","port":8080}`))

	// Assert that RegisterRoutes returned
	Expect(returned).To(Receive())
}

func subscribeToRegisterEvents(callback func(msg *yagnats.Message)) (registerChannel chan string) {
	registerChannel = make(chan string)

	fmt.Println("Subscribing to register...")
	go client.Subscribe("router.register", callback)

	return
}

func subscribeToUnregisterEvents(callback func(msg *yagnats.Message)) (unregisterChannel chan bool) {
	unregisterChannel = make(chan bool)

	fmt.Println("Subscribing to unregister...")
	go client.Subscribe("router.unregister", callback)

	return
}
