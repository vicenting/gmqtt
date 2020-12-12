package admin

import (
	"container/list"
	"errors"
	"sync"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/server"
)

type store struct {
	clientMu            sync.Mutex
	clientList          *quickList
	subMu               sync.Mutex
	subscriptions       *quickList
	config              config.Config
	statsReader         server.StatsReader
	subscriptionService server.SubscriptionService
}

func newStore(statsReader server.StatsReader) *store {
	return &store{
		clientList:    newQuickList(),
		subscriptions: newQuickList(),
		statsReader:   statsReader,
	}
}

func (s *store) addSubscription(clientID string, sub *gmqtt.Subscription) {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	subInfo := &Subscription{
		TopicName:         sub.GetFullTopicName(),
		Id:                sub.ID,
		Qos:               uint32(sub.QoS),
		NoLocal:           sub.NoLocal,
		RetainAsPublished: sub.RetainAsPublished,
		RetainHandling:    uint32(sub.RetainHandling),
		ClientId:          clientID,
	}
	key := clientID + "_" + sub.GetFullTopicName()
	s.subscriptions.set(key, subInfo)

}

func (s *store) removeSubscription(clientID string, topicName string) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	s.subscriptions.remove(clientID + "_" + topicName)
}

var ErrNotFound = errors.New("not found")

type quickList struct {
	index map[string]*list.Element
	rows  *list.List
}

func newQuickList() *quickList {
	return &quickList{
		index: make(map[string]*list.Element),
		rows:  list.New(),
	}
}
func (q *quickList) set(id string, value interface{}) {
	if e, ok := q.index[id]; ok {
		e.Value = value
	} else {
		elem := q.rows.PushBack(value)
		q.index[id] = elem
	}
}
func (q *quickList) remove(id string) *list.Element {
	elem := q.index[id]
	if elem != nil {
		q.rows.Remove(elem)
	}
	delete(q.index, id)
	return elem
}
func (q *quickList) getByID(id string) *list.Element {
	return q.index[id]
}
func (q *quickList) iterate(fn func(elem *list.Element), offset, n uint) {
	if q.rows.Len() < int(offset) {
		return
	}
	var i uint
	for e := q.rows.Front(); e != nil; e = e.Next() {
		if i >= offset && i < offset+n {
			fn(e)
		}
		if i == offset+n {
			break
		}
		i++
	}
}

func (s *store) addClient(client server.Client) {
	c := newClientInfo(client)
	s.clientMu.Lock()
	s.clientList.set(c.ClientId, c)
	s.clientMu.Unlock()
}

func (s *store) setClientDisconnected(clientID string) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	l := s.clientList.getByID(clientID)
	if l == nil {
		return
	}
	l.Value.(*Client).DisconnectedAt = timestamppb.Now()
}

func (s *store) removeClient(clientID string) {
	s.clientMu.Lock()
	s.clientList.remove(clientID)
	s.clientMu.Unlock()
}

// GetClientByID returns the client information for the given client id.
func (s *store) GetClientByID(clientID string) *Client {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	c := s.getClientByIDLocked(clientID)
	fillClientInfo(c, s.statsReader)
	return c
}

func newClientInfo(client server.Client) *Client {
	clientOptions := client.ClientOptions()
	rs := &Client{
		ClientId:       clientOptions.ClientID,
		Username:       clientOptions.Username,
		KeepAlive:      int32(clientOptions.KeepAlive),
		Version:        int32(client.Version()),
		RemoteAddr:     client.Connection().RemoteAddr().String(),
		LocalAddr:      client.Connection().LocalAddr().String(),
		ConnectedAt:    timestamppb.New(client.ConnectedAt()),
		DisconnectedAt: nil,
		SessionExpiry:  clientOptions.SessionExpiry,
		MaxInflight:    uint32(clientOptions.MaxInflight),
		MaxQueue:       uint32(clientOptions.ReceiveMax),
	}
	return rs
}

func (s *store) getClientByIDLocked(clientID string) *Client {
	if i := s.clientList.getByID(clientID); i != nil {
		return i.Value.(*Client)
	} else {
		return nil
	}
}

func fillClientInfo(c *Client, stsReader server.StatsReader) {
	if c == nil {
		return
	}
	sts, ok := stsReader.GetClientStats(c.ClientId)
	if !ok {
		return
	}
	c.SubscriptionsCurrent = uint32(sts.SubscriptionStats.SubscriptionsCurrent)
	c.SubscriptionsTotal = uint32(sts.SubscriptionStats.SubscriptionsTotal)
	c.PacketsReceivedBytes = sts.PacketStats.BytesReceived.Total
	c.PacketsReceivedNums = sts.PacketStats.ReceivedTotal.Total
	c.PacketsSendBytes = sts.PacketStats.BytesSent.Total
	c.PacketsSendNums = sts.PacketStats.SentTotal.Total
	c.MessageDropped = sts.MessageStats.GetDroppedTotal()
	c.InflightLen = uint32(sts.MessageStats.InflightCurrent)
	c.QueueLen = uint32(sts.MessageStats.QueuedCurrent)
}

// GetClients
func (s *store) GetClients(page, pageSize uint) (rs []*Client, total uint32, err error) {
	rs = make([]*Client, 0)
	fn := func(elem *list.Element) {
		c := elem.Value.(*Client)
		fillClientInfo(c, s.statsReader)
		rs = append(rs, elem.Value.(*Client))
	}
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	offset, n := getOffsetN(page, pageSize)
	s.clientList.iterate(fn, offset, n)
	return rs, uint32(s.clientList.rows.Len()), nil
}

// GetSubscriptions
func (s *store) GetSubscriptions(page, pageSize uint) (rs []*Subscription, total uint32, err error) {
	rs = make([]*Subscription, 0)
	fn := func(elem *list.Element) {
		rs = append(rs, elem.Value.(*Subscription))
	}
	s.subMu.Lock()
	defer s.subMu.Unlock()
	offset, n := getOffsetN(page, pageSize)
	s.subscriptions.iterate(fn, offset, n)
	return rs, uint32(s.subscriptions.rows.Len()), nil
}

func getOffsetN(page, pageSize uint) (offset, n uint) {
	offset = (page - 1) * pageSize
	n = pageSize
	return
}
