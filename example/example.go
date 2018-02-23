package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/tgulacsi/go-xmlrpc"
)

var (
	flagServe = flag.String("serve", "", "host:port to serve on")
	// DefaultXMLRpcPath is /example
	DefaultXMLRpcPath = "/example"
)

// Args is the struct for the input
type Args struct {
	A, B int
}

// Quotient is the result of the division
type Quotient struct {
	Quo, Rem int
}

// Arith is the holder for the operations
type Arith struct{}

// Multiply retruns args.A * args.B
func (t *Arith) Multiply(args *Args, reply *int) error {
	*reply = args.A * args.B
	return nil
}

// Divide returns args.A / args.B and args.A % args.B
func (t *Arith) Divide(args *Args, quo *Quotient) error {
	if args.B == 0 {
		return errors.New("divide by zero")
	}
	quo.Quo = args.A / args.B
	quo.Rem = args.A % args.B
	return nil
}

func main() {
	flag.Parse()

	if *flagServe != "" {
		arith := new(Arith)
		srv := xmlrpc.NewServer()
		srv.Register(arith)
		srv.SetHTTPHandler("/")
		log.Printf("serving on http://%s"+DefaultXMLRpcPath, *flagServe)
		log.Fatal(http.ListenAndServe(*flagServe, nil))
		return
	}

	if flag.NArg() == 0 {
		log.Println("calling http://your-blog.example.com/xmlrpc.php")
		res, fault, e := xmlrpc.Call(
			"http://your-blog.example.com/xmlrpc.php",
			"metaWeblog.getRecentPosts",
			"blog-id",
			"user-id",
			"password",
			10)
		if e != nil {
			log.Fatal(e)
		}
		if fault != nil {
			log.Fatalf("remote error: %s", fault)
		}
		log.Printf("got %#v", res)
		/*
			for _, p := range res.(xmlrpc.Array) {
				for k, v := range p.(xmlrpc.Struct) {
					fmt.Printf("%s=%v\n", k, v)
				}
				fmt.Println()
			}
		*/
		return
	}

	// client
	if flag.NArg() < 2 {
		log.Printf("at least two arguments required: the target url, and the method name to be called")
		os.Exit(1)
	}
	var err error
	url := flag.Arg(0)
	method := flag.Arg(1)
	args := new(Args)
	if flag.NArg() > 2 {
		if args.A, err = strconv.Atoi(flag.Arg(2)); err != nil {
			log.Fatalf("int needed instead of %q: %s", flag.Arg(2), err)
		}
		if flag.NArg() > 3 {
			if args.B, err = strconv.Atoi(flag.Arg(3)); err != nil {
				log.Fatalf("int needed instead of %q: %s", flag.Arg(3), err)
			}
		}
	}
	log.Printf("creating client for %s/%s", url, DefaultXMLRpcPath)
	client, err := xmlrpc.DialHTTPPath("tcp", url, DefaultXMLRpcPath)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("calling method %q%s at url %q", method, args, url)
	var (
		reply    int
		quotient Quotient
	)
	switch method {
	case "/", "divide":
		if err = client.Call("Arith.Divide", args, &quotient); err != nil {
			log.Fatalf("error calling Arith.Divide: %s", err)
		}
		log.Printf("%s(%d, %d) = %d", method, args.A, args.B, quotient)
	case "*", "multiply":
		if err = client.Call("Arith.Multiply", args, &reply); err != nil {
			log.Fatalf("error calling Arith.Multiply: %s", err)
		}
		log.Printf("%s(%d, %d) = %d", method, args.A, args.B, reply)
	default:
		log.Fatalf("unknown method %q", method)
	}
}
