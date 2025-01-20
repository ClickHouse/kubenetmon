# ipconv

This library provides simple conversion between `net.IP` and integer (`net.IP <--> int`). As new feature, library now contains extension of `net.ParseIP` which returns also byte length of IP address on input.

I hope it will serve you well.

## Example

```go
package main

import (
    "fmt"
    "net"
    "github.com/PraserX/ipconv"
)

func main() {
    if ip, version, err := net.ParseIP("192.168.1.1"); err != nil && version == 4 {
        fmt.Println(ipconv.IPv4ToInt(ip))
    }
}
```