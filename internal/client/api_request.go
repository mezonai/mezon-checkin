package client

import (
	"encoding/binary"
	"fmt"
	"log"
	"time"

	rtapi "mezon-checkin-bot/mezon-protobuf/go/rtapi"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

const (
	rawFramePrefix  = 0xff
	rawHeaderLength = 7
	rawCodeFin      = 0xffff
)

type apiSocketResponse struct {
	code uint32
	body []byte
}

// sendApiRequest invokes a Mezon REST API over the open WebSocket connection.
func (c *MezonClient) sendApiRequest(apiName string, body []byte, timeout time.Duration) ([]byte, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("websocket not connected")
	}

	apiIndex, ok := apiIndexFromName(apiName)
	if !ok {
		return nil, fmt.Errorf("unknown API %q", apiName)
	}

	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()
	if conn == nil {
		return nil, fmt.Errorf("websocket connection is nil")
	}

	cid := c.generateCID()
	envelope := &rtapi.Envelope{
		Cid: cid,
		Message: &rtapi.Envelope_ApiRequestEvent{
			ApiRequestEvent: &rtapi.ApiRequestEvent{
				ApiIndex: apiIndex,
				ApiName:  apiName,
				Body:     body,
			},
		},
	}

	responseChan := make(chan *apiSocketResponse, 1)
	c.apiCidMu.Lock()
	c.apiCidHandlers[cid] = responseChan
	c.apiCidMu.Unlock()

	defer func() {
		c.apiCidMu.Lock()
		delete(c.apiCidHandlers, cid)
		c.apiCidMu.Unlock()
	}()

	data, err := proto.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal api request: %w", err)
	}

	if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return nil, fmt.Errorf("write api request: %w", err)
	}

	if c.verbose {
		logAPIRequest(apiName, cid, len(body))
	}

	select {
	case resp := <-responseChan:
		if resp == nil {
			return nil, fmt.Errorf("empty API response for %s", apiName)
		}
		if resp.code != 0 {
			return nil, fmt.Errorf("API %s failed with code %d", apiName, resp.code)
		}
		return resp.body, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for API %s response", apiName)
	case <-c.ctx.Done():
		return nil, fmt.Errorf("context cancelled")
	}
}

func logAPIRequest(apiName string, cid int32, bodyLen int) {
	log.Printf("📤 API request %s CID=%d (%d bytes body)", apiName, cid, bodyLen)
}

func (c *MezonClient) processRawAPIFrame(data []byte) {
	if len(data) < rawHeaderLength || data[0] != rawFramePrefix {
		return
	}

	cid := int32(binary.BigEndian.Uint16(data[1:3]))
	meta := binary.BigEndian.Uint32(data[3:7])
	code := meta >> 16
	fin := meta & 0xffff
	payload := append([]byte(nil), data[rawHeaderLength:]...)

	if fin != rawCodeFin {
		c.apiStreamsMu.Lock()
		c.apiStreams[cid] = append(c.apiStreams[cid], payload)
		c.apiStreamsMu.Unlock()
		return
	}

	c.apiStreamsMu.Lock()
	chunks := c.apiStreams[cid]
	delete(c.apiStreams, cid)
	c.apiStreamsMu.Unlock()

	total := len(payload)
	for _, chunk := range chunks {
		total += len(chunk)
	}
	body := make([]byte, 0, total)
	for _, chunk := range chunks {
		body = append(body, chunk...)
	}
	body = append(body, payload...)

	c.deliverAPIResponse(cid, code, body)
}

func (c *MezonClient) deliverAPIResponse(cid int32, code uint32, body []byte) {
	ch, fullCID, ok := c.takeAPICidHandler(cid)
	if !ok {
		if c.verbose {
			log.Printf("⚠️  No handler for API response CID=%d", cid)
		}
		return
	}

	resp := &apiSocketResponse{code: code, body: body}
	select {
	case ch <- resp:
	default:
		log.Printf("⚠️  API response channel timeout for CID=%d (registered as %d)", cid, fullCID)
	}
}

func (c *MezonClient) takeAPICidHandler(cid int32) (chan *apiSocketResponse, int32, bool) {
	c.apiCidMu.Lock()
	defer c.apiCidMu.Unlock()

	if ch, ok := c.apiCidHandlers[cid]; ok {
		delete(c.apiCidHandlers, cid)
		return ch, cid, true
	}

	for k, ch := range c.apiCidHandlers {
		if int32(uint16(k)) == cid {
			delete(c.apiCidHandlers, k)
			return ch, k, true
		}
	}
	return nil, 0, false
}

func (c *MezonClient) clearAPIState() {
	c.apiCidMu.Lock()
	c.apiCidHandlers = make(map[int32]chan *apiSocketResponse)
	c.apiCidMu.Unlock()

	c.apiStreamsMu.Lock()
	c.apiStreams = make(map[int32][][]byte)
	c.apiStreamsMu.Unlock()
}
