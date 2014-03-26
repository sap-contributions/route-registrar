package registrar

import (
	"os"
	"os/signal"
	"syscall"
	"fmt"

	"github.com/cloudfoundry/gibson"
	"github.com/cloudfoundry/yagnats"

	"github.com/cloudfoundry-incubator/route-registrar/config"
)

type Registrar struct {
	Config config.Config
	SignalChannel chan os.Signal
}

func NewRegistrar(clientConfig config.Config) *Registrar {
	registrar := new(Registrar)
	registrar.Config = clientConfig
	registrar.SignalChannel = make(chan os.Signal, 1)
	return registrar
}

func(registrar *Registrar) RegisterRoutes() {
	messageBus := yagnats.NewClient()
	connectionInfo := yagnats.ConnectionInfo{
		registrar.Config.MessageBusServer.Host,
		registrar.Config.MessageBusServer.User,
		registrar.Config.MessageBusServer.Password,
	}

	err := messageBus.Connect(&connectionInfo)
	if err != nil {
		fmt.Println("Error connecting: ", err)
		panic("Failed to connect to NATS bus.")
	}
	fmt.Printf("Connected to NATS at %+v\n", registrar.Config.MessageBusServer)

	client := gibson.NewCFRouterClient(registrar.Config.ExternalIp, messageBus)

	// set up periodic registration
	client.Greet()

	client.Register(registrar.Config.Port, registrar.Config.ExternalHost)

	done := make(chan bool)
	registrar.registerSignalHandler(done, client)

	select {
	case <- done:
		return
	}
}

func(registrar *Registrar) registerSignalHandler(done chan bool, client *gibson.CFRouterClient) {

	go func() {
		select {
		case <-registrar.SignalChannel:
			fmt.Println("recieved signal")
			client.Unregister(registrar.Config.Port, registrar.Config.ExternalHost)
			done <- true
		}
	}()

	signal.Notify(registrar.SignalChannel, syscall.SIGINT, syscall.SIGTERM)
}
