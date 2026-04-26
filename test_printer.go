package main

import (
    "fmt"
    "net"
    "time"
)

func main() {
    // Test if we can connect to 192.168.192.12:9100
    addr := fmt.Sprintf("%s:%d", "192.168.192.12", 9100)
    conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
    if err != nil {
        fmt.Printf("Connection error: %v\n", err)
        return
    }
    defer conn.Close()
    fmt.Println("Connected successfully!")
}
