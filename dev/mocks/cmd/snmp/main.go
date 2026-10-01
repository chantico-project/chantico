package main

import (
	"log"
	"net"

	"github.com/gosnmp/gosnmp"

	"chantico/dev/mocks/internal/simulation"
)

var (
	address = simulation.EnvString("MOCK_SNMP_ADDRESS", ":1161")
	walk    = simulation.NewWalk(simulation.BoundsFromEnv("MOCK_SNMP_VALUE", simulation.Bounds{
		Min:      200,
		Max:      800,
		Start:    500,
		Variance: 10,
	}))
)

func main() {
	conn, err := net.ListenPacket("udp", address)
	if err != nil {
		log.Fatalf("Error starting SNMP listener: %v", err)
	}
	log.Printf("Listening on %s", address)

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
				variables[i].Value = int(walk.Next(variables[i].Name))
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
