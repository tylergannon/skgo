package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	for _, p := range []int{56611, 56612, 56621, 56613, 56623, 51389, 51401, 51841} {
		c, e := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p), 200*time.Millisecond)
		if e == nil {
			c.Close()
			panic(fmt.Sprintf("owned port still open%d", p))
		}
		fmt.Printf("CLOSED ownedport%d\n", p)
	}
	if e := os.RemoveAll("ephemeral/experiments/20261015-coordinator-integration/runtime"); e != nil {
		panic(e)
	}
	fmt.Println("REMOVED owned runtime only")
}
