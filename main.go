package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strings"
)

type DNSRecord struct {
	name       []byte
	type_      RecordType
	class_     ClassType
	TTL        uint32
	data       []byte
	dataOffset int
}

type DNSHeader struct {
	id              uint16
	flags           uint16
	num_questions   uint16
	num_answers     uint16
	num_authorities uint16
	num_additionals uint16
}

func (h DNSHeader) ToBytes() []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint16(buf[0:2], h.id)
	binary.BigEndian.PutUint16(buf[2:4], h.flags)
	binary.BigEndian.PutUint16(buf[4:6], h.num_questions)
	binary.BigEndian.PutUint16(buf[6:8], h.num_answers)
	binary.BigEndian.PutUint16(buf[8:10], h.num_authorities)
	binary.BigEndian.PutUint16(buf[10:12], h.num_additionals)
	return buf
}

type RecordType uint16

const (
	A     RecordType = 1
	NS    RecordType = 2
	CNAME RecordType = 5
	AAAA  RecordType = 28
)

func (r RecordType) String() string {
	switch r {
	case A:
		return "A"
	case NS:
		return "NS"
	case CNAME:
		return "CNAME"
	case AAAA:
		return "AAAA"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", r)
	}
}

type ClassType uint16

const (
	IN ClassType = 1
	CH ClassType = 3
	HS ClassType = 4
)

func (c ClassType) String() string {
	switch c {
	case IN:
		return "IN"
	case CH:
		return "CH"
	case HS:
		return "HS"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", c)
	}
}

type DNSQuestion struct {
	name   []byte
	type_  RecordType
	class_ ClassType
}

func (q DNSQuestion) ToBytes() []byte {
	buf := make([]byte, len(q.name)+4)
	copy(buf[:], q.name)
	binary.BigEndian.PutUint16(buf[len(q.name):], uint16(q.type_))
	binary.BigEndian.PutUint16(buf[len(q.name)+2:], uint16(q.class_))
	return buf
}

func encode_dns_name(domain string) []byte {
	var buf []byte
	for part := range strings.SplitSeq(domain, ".") {
		buf = append(buf, byte(len(part)))
		buf = append(buf, part...)
	}
	buf = append(buf, 0)
	return buf
}

func build_query(domain string, record_type RecordType) []byte {
	name := encode_dns_name(domain)
	id := uint16(123) // random
	RECURSION_DESIRED := 1 << 8

	header := DNSHeader{
		id:            id,
		num_questions: 1,
		flags:         uint16(RECURSION_DESIRED),
	}

	question := DNSQuestion{
		name:   name,
		type_:  record_type,
		class_: IN,
	}

	return append(header.ToBytes(), question.ToBytes()...)
}

func parse_header(data []byte) DNSHeader {
	return DNSHeader{
		id:              binary.BigEndian.Uint16(data[0:2]),
		flags:           binary.BigEndian.Uint16(data[2:4]),
		num_questions:   binary.BigEndian.Uint16(data[4:6]),
		num_answers:     binary.BigEndian.Uint16(data[6:8]),
		num_authorities: binary.BigEndian.Uint16(data[8:10]),
		num_additionals: binary.BigEndian.Uint16(data[10:12]),
	}
}

func parse_name(data []byte, offset int) ([]byte, int) {
	var parts [][]byte
	for {
		if data[offset]&0xC0 == 0xC0 {
			ptr := int(binary.BigEndian.Uint16(data[offset:])) & 0x3FFF
			resolved, _ := parse_name(data, ptr)
			parts = append(parts, resolved)
			return bytes.Join(parts, []byte(".")), offset + 2
		}
		length := int(data[offset])
		offset++
		if length == 0 {
			break
		}
		parts = append(parts, data[offset:offset+length])
		offset += length
	}
	name := bytes.Join(parts, []byte("."))
	return name, offset
}

func parse_question(data []byte, offset int) (DNSQuestion, int) {
	name, newOffset := parse_name(data, offset)
	return DNSQuestion{
		name:   name,
		type_:  RecordType(binary.BigEndian.Uint16(data[newOffset:])),
		class_: ClassType(binary.BigEndian.Uint16(data[newOffset+2:])),
	}, newOffset + 4
}

func (r DNSRecord) data_to_str(full []byte) string {
	switch r.type_ {
	case A, AAAA:
		return net.IP(r.data).String()
	case NS, CNAME:
		name, _ := parse_name(full, r.dataOffset)
		return string(name)
	default:
		return fmt.Sprintf("%x", r.data)
	}
}

func parse_record(data []byte, offset int) (DNSRecord, int) {
	name, offset := parse_name(data, offset)
	rec := DNSRecord{
		name:       name,
		type_:      RecordType(binary.BigEndian.Uint16(data[offset:])),
		class_:     ClassType(binary.BigEndian.Uint16(data[offset+2:])),
		TTL:        binary.BigEndian.Uint32(data[offset+4:]),
		dataOffset: offset + 10,
	}
	offset += 8
	rdLen := binary.BigEndian.Uint16(data[offset:])
	offset += 2
	rec.data = data[offset : offset+int(rdLen)]
	offset += int(rdLen)
	return rec, offset
}

func main() {
	resolver := "1.1.1.1:53"
	var domain string
	recordType := A

	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "@") {
			ip := arg[1:]
			if ip == "" {
				fmt.Fprintf(os.Stderr, "usage: dnsq [@resolver] <domain> [A|AAAA|NS|CNAME]\n")
				os.Exit(1)
			}
			resolver = net.JoinHostPort(ip, "53")
		} else {
			switch strings.ToUpper(arg) {
			case "A":
				recordType = A
			case "AAAA":
				recordType = AAAA
			case "NS":
				recordType = NS
			case "CNAME":
				recordType = CNAME
			default:
				domain = arg
			}
		}
	}

	if domain == "" {
		fmt.Fprintf(os.Stderr, "usage: dnsq [@resolver] <domain> [A|AAAA|NS|CNAME]\n")
		os.Exit(1)
	}

	query := build_query(domain, recordType)

	conn, err := net.Dial("udp", resolver)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	_, err = conn.Write(query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	response := make([]byte, 512)
	n, err := conn.Read(response)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	response = response[:n]

	header := parse_header(response)

	_, offset := parse_question(response, 12)

	for i := uint16(0); i < header.num_answers; i++ {
		record, newOffset := parse_record(response, offset)
		offset = newOffset

		fmt.Printf("%s %s %s %s %d\n",
			string(record.name),
			record.data_to_str(response),
			record.type_,
			record.class_,
			record.TTL,
		)
	}
}

