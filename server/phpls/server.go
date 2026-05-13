// Package phpls contains the JSON-RPC 2.0 stdio transport and LSP request
// handler for the phpls language server. It is separate from the `lsp` package
// (protocol types only) to avoid import cycles with the providers package.
package phpls

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/ayanozturk/vscode-php-strom/lsp"
)

// Server handles JSON-RPC 2.0 over stdin/stdout.
type Server struct {
	in      io.Reader
	out     io.Writer
	handler *Handler
	reader  *bufio.Reader
}

// NewServer creates a Server that reads from in and writes to out.
func NewServer(in io.Reader, out io.Writer) *Server {
	s := &Server{in: in, out: out}
	s.reader = bufio.NewReader(in)
	s.handler = NewHandler(s)
	return s
}

// Run is the main read-dispatch-write loop. It blocks until EOF or a fatal error.
func (s *Server) Run() error {
	for {
		msg, err := s.readMessage()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			log.Printf("read error: %v", err)
			return err
		}
		go s.dispatch(msg)
	}
}

// Notify sends an LSP notification (no ID, no response expected).
func (s *Server) Notify(method string, params interface{}) {
	n := lsp.NotificationMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	if err := s.writeJSON(n); err != nil {
		log.Printf("notify write error: %v", err)
	}
}

// ─── Internal ─────────────────────────────────────────────────────────────────

func (s *Server) dispatch(raw []byte) {
	var msg lsp.RequestMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		log.Printf("unmarshal request: %v", err)
		return
	}

	if msg.ID == nil {
		// Notification
		s.handler.HandleNotification(msg.Method, msg.Params)
		return
	}

	// Request
	result, rpcErr := s.handler.HandleRequest(msg.Method, msg.Params)
	resp := lsp.ResponseMessage{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result:  result,
		Error:   rpcErr,
	}
	if err := s.writeJSON(resp); err != nil {
		log.Printf("write response: %v", err)
	}
}

func (s *Server) readMessage() ([]byte, error) {
	var contentLength int
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			n, err := strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length: %w", err)
			}
			contentLength = n
		}
	}
	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(s.reader, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (s *Server) writeJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	_, err = fmt.Fprint(s.out, header)
	if err != nil {
		return err
	}
	_, err = s.out.Write(data)
	return err
}
