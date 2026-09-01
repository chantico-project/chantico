package main

import (
	"log"
	"math/rand"
	"net"

	"github.com/gosnmp/gosnmp"
)

func main() {
	addr := ":1161"
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Fatalf("Error starting SNMP listener: %v", err)
	}
	log.Printf("Listening on %s", addr)

	buf := make([]byte, 2048)
	for {
		n, remoteAddr, err := conn.ReadFrom(buf)
		if err != nil {
			log.Printf("Read error: %v", err)
			continue
		}

		packet, err := gosnmp.Default.SnmpDecodePacket(buf[:n])
		if err != nil {
			log.Printf("Decode error: %s", err.Error())
			continue
		}

		if packet.PDUType == gosnmp.GetRequest {
			variables := packet.Variables
			for i := range variables {
				variables[i].Type = gosnmp.Integer
				variables[i].Value = rand.Intn(1000)
			}

			response := &gosnmp.SnmpPacket{
				Version:   packet.Version,
				Community: packet.Community,
				PDUType:   gosnmp.GetResponse,
				RequestID: packet.RequestID,
				Variables: variables,
			}

			out, err := response.MarshalMsg()
			if err != nil {
				log.Printf("Encode error: %v", err)
				continue
			}

			_, err = conn.WriteTo(out, remoteAddr)
			if err != nil {
				log.Printf("Write error: %v", err)
			}
		}
	}
}
