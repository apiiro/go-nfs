package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/willscott/memphis"

	nfs "github.com/willscott/go-nfs"
	nfshelper "github.com/willscott/go-nfs/helpers"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: osview /path/to/folder\n")
		return
	}
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		fmt.Printf("Failed to listen: %v\n", err)
		return
	}
	fmt.Printf("Server running at %s\n", listener.Addr())

	fs := memphis.FromOS(os.Args[1])
	bfs := fs.AsBillyFS(0, 0)

	handler := nfshelper.NewNullAuthHandler(bfs)
	binaryHandler := nfshelper.NewBinaryHandler(handler, bfs)
	fmt.Printf("%v", nfs.Serve(listener, binaryHandler, &log.Logger{}, &log.Logger{}))
}
