package flow

import (
	"code.google.com/p/go.net/websocket"
	"github.com/nu7hatch/gouuid"
	"log"
	"net"
	"net/http"
)

type protocolHandler func(*websocket.Conn, interface{})

// Runtime is a NoFlo-compatible runtime implementing the FBP protocol
type Runtime struct {
	// Unique runtime ID for use with Flowhub
	id string
	// Protocol command handlers
	handlers map[string]protocolHandler
	// Graphs created at runtime and exposed as components
	graphs map[string]*Graph
	// Main graph ID
	mainId string
	// Main graph
	main *Graph
	// Websocket server onReady signal
	ready chan struct{}
	// Websocket server onShutdown signal
	done chan struct{}
}

// Register command handlers
func (r *Runtime) Init() {
	uv4, err := uuid.NewV4()
	if err != nil {
		log.Println(err.Error())
	}
	r.id = uv4.String()
	r.done = make(chan struct{})
	r.ready = make(chan struct{})
	r.handlers = make(map[string]protocolHandler)
	r.handlers["runtime.getruntime"] = func(ws *websocket.Conn, payload interface{}) {
		websocket.JSON.Send(ws, runtimeInfo{"goflow",
			"0.4",
			[]string{"protocol:runtime",
				"protocol:graph",
				"protocol:component",
				"protocol:network",
				"component:getsource"},
			r.id,
		})
	}
	r.handlers["graph.clear"] = func(ws *websocket.Conn, payload interface{}) {
		msg := payload.(clearGraph)
		r.graphs[msg.Id] = new(Graph)
		r.graphs[msg.Id].InitGraphState()
		if msg.Main {
			r.mainId = msg.Id
			r.main = r.graphs[msg.Id]
		}
		// TODO register as a component
		// TODO send component.component back
	}
	r.handlers["graph.addnode"] = func(ws *websocket.Conn, payload interface{}) {
		msg := payload.(addNode)
		r.graphs[msg.Graph].AddNew(msg.Component, msg.Id)
	}
	r.handlers["graph.removenode"] = func(ws *websocket.Conn, payload interface{}) {
		msg := payload.(removeNode)
		r.graphs[msg.Graph].Remove(msg.Id)
	}
	r.handlers["graph.renamenode"] = func(ws *websocket.Conn, payload interface{}) {
		msg := payload.(renameNode)
		r.graphs[msg.Graph].Rename(msg.From, msg.To)
	}
	r.handlers["graph.changenode"] = func(ws *websocket.Conn, payload interface{}) {
		// Currently unsupported
	}
	r.handlers["graph.addedge"] = func(ws *websocket.Conn, payload interface{}) {
		msg := payload.(addEdge)
		r.graphs[msg.Graph].Connect(msg.Src.Node, msg.Src.Port, msg.Tgt.Node, msg.Tgt.Port)
	}
	r.handlers["graph.removedge"] = func(ws *websocket.Conn, payload interface{}) {
		msg := payload.(removeEdge)
		r.graphs[msg.Graph].Disconnect(msg.Src.Node, msg.Src.Port, msg.Tgt.Node, msg.Tgt.Port)
	}
	r.handlers["graph.changeedge"] = func(ws *websocket.Conn, payload interface{}) {
		// Currently unsupported
	}
	r.handlers["graph.addinitial"] = func(ws *websocket.Conn, payload interface{}) {
		msg := payload.(addInitial)
		r.graphs[msg.Graph].AddIIP(msg.Src.Data, msg.Tgt.Node, msg.Tgt.Port)
	}
	r.handlers["graph.removeinitial"] = func(ws *websocket.Conn, payload interface{}) {
		msg := payload.(removeInitial)
		r.graphs[msg.Graph].RemoveIIP(msg.Tgt.Node, msg.Tgt.Port)
	}
	r.handlers["graph.addinport"] = func(ws *websocket.Conn, payload interface{}) {
		msg := payload.(addPort)
		r.graphs[msg.Graph].MapInPort(msg.Public, msg.Node, msg.Port)
	}
	r.handlers["graph.removeinport"] = func(ws *websocket.Conn, payload interface{}) {
		msg := payload.(removePort)
		r.graphs[msg.Graph].UnsetInPort(msg.Public)
		r.graphs[msg.Graph].UnmapInPort(msg.Public)
	}
	r.handlers["graph.renameinport"] = func(ws *websocket.Conn, payload interface{}) {
		msg := payload.(renamePort)
		r.graphs[msg.Graph].RenameInPort(msg.From, msg.To)
	}
	r.handlers["graph.addoutport"] = func(ws *websocket.Conn, payload interface{}) {
		msg := payload.(addPort)
		r.graphs[msg.Graph].MapOutPort(msg.Public, msg.Node, msg.Port)
	}
	r.handlers["graph.removeoutport"] = func(ws *websocket.Conn, payload interface{}) {
		msg := payload.(removePort)
		r.graphs[msg.Graph].UnsetOutPort(msg.Public)
		r.graphs[msg.Graph].UnmapOutPort(msg.Public)
	}
	r.handlers["graph.renameoutport"] = func(ws *websocket.Conn, payload interface{}) {
		msg := payload.(renamePort)
		r.graphs[msg.Graph].RenameOutPort(msg.From, msg.To)
	}
	r.handlers["component.list"] = func(ws *websocket.Conn, payload interface{}) {
		// TODO
	}
}

// Id returns runtime's UUID v4
func (r *Runtime) Id() string {
	return r.id
}

// Ready returns a channel which is closed when the runtime is ready to work
func (r *Runtime) Ready() chan struct{} {
	return r.ready
}

// Stop tells the runtime to shut down
func (r *Runtime) Stop() {
	close(r.done)
}

func (r *Runtime) Handle(ws *websocket.Conn) {
	defer func() {
		err := ws.Close()
		if err != nil {
			log.Println(err.Error())
		}
	}()
	var msg Message
	if err := websocket.JSON.Receive(ws, &msg); err != nil {
		log.Println(err.Error())
		return
	}
	handler, exists := r.handlers[msg.Protocol+"."+msg.Command]
	if !exists {
		log.Printf("Unknown command: %s.%s\n", msg.Protocol, msg.Command)
		return
	}
	handler(ws, msg.Payload)
}

func (r *Runtime) Listen(address string) {
	http.Handle("/", websocket.Handler(r.Handle))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalln(err.Error())
	}

	go func() {
		err = http.Serve(listener, nil)
		if err != nil {
			log.Fatalln(err.Error())
		}
	}()
	close(r.ready)

	// Wait for termination signal
	<-r.done
	listener.Close()
}
