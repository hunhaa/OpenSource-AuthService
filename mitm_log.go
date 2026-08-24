//go:build ignore

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// MITM 日志代理：监听 19090，转发到 18080，把双向所有字节都 print 出来
// 用法: go run mitm_log.go &
//       curl --preproxy http://127.0.0.1:19090 -x http://user:pass@ip:port https://httpbin.org/ip

var (
	upstream = flag.String("up", "127.0.0.1:18080", "real proxy upstream")
	listen   = flag.String("l", ":19090", "listen addr")
	counter  int64
)

func hexdump(prefix string, b []byte) string {
	out := new(strings.Builder)
	// 先打印可读字符，遇到不可读就用点
	 printable := func(b byte) byte {
		if b >= 32 && b <= 126 || b == '\r' || b == '\n' || b == '\t' {
			return b
		}
		return '.'
	 }
	 cur := ""
	 for i, by := range b {
		if i%64 == 0 && i > 0 {
			fmt.Fprintf(out, "%s  | %s\n", prefix, cur)
			cur = ""
		}
		cur += string(printable(by))
	 }
	 if cur != "" {
		fmt.Fprintf(out, "%s  | %s\n", prefix, cur)
	 }
	 return out.String()
}

func handleConn(in net.Conn) {
	id := atomic.AddInt64(&counter, 1)
	pfx := fmt.Sprintf("[%d]", id)
	defer in.Close()

	up, err := net.DialTimeout("tcp", *upstream, 10*time.Second)
	if err != nil {
		log.Printf("%s FAIL dial upstream: %v", pfx, err)
		return
	}
	defer up.Close()
	log.Printf("%s CONNECTED in=%s up=%s", pfx, in.RemoteAddr(), up.RemoteAddr())

	done := make(chan struct{}, 2)

	go func() {
		buf := make([]byte, 65536)
		for {
			in.SetReadDeadline(time.Now().Add(30 * time.Second))
			n, err := in.Read(buf)
			if n > 0 {
				fmt.Fprintf(os.Stderr, "\n===== %s C->S (%d bytes) =====\n%s", pfx, n, hexdump(pfx+" ", buf[:n]))
				if _, werr := up.Write(buf[:n]); werr != nil {
					log.Printf("%s C->S write err: %v", pfx, werr)
					done <- struct{}{}
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					log.Printf("%s C->S read err: %v", pfx, err)
				}
				done <- struct{}{}
				return
			}
		}
	}()

	go func() {
		buf := make([]byte, 65536)
		for {
			up.SetReadDeadline(time.Now().Add(30 * time.Second))
			n, err := up.Read(buf)
			if n > 0 {
				fmt.Fprintf(os.Stderr, "\n===== %s S->C (%d bytes) =====\n%s", pfx, n, hexdump(pfx+" ", buf[:n]))
				if _, werr := in.Write(buf[:n]); werr != nil {
					log.Printf("%s S->C write err: %v", pfx, werr)
					done <- struct{}{}
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					log.Printf("%s S->C read err: %v", pfx, err)
				}
				done <- struct{}{}
				return
			}
		}
	}()

	<-done
	<-done
	log.Printf("%s CLOSED", pfx)
}

func main() {
	flag.Parse()
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("MITM proxy listening on %s -> upstream %s", *listen, *upstream)
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go handleConn(c)
	}
}
