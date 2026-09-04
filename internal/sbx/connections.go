package sbx

import (
	"sort"
	"strings"
	"time"
)

type ConnectionTable struct {
	active      map[string]Connection
	closed      map[string]Connection
	closedLimit int
	interval    time.Duration
}

func NewConnectionTable(closedLimit int, interval time.Duration) *ConnectionTable {
	if closedLimit < 0 {
		closedLimit = 0
	}
	if interval <= 0 {
		interval = time.Second
	}
	return &ConnectionTable{
		active:      make(map[string]Connection),
		closed:      make(map[string]Connection),
		closedLimit: closedLimit,
		interval:    interval,
	}
}

func (t *ConnectionTable) Apply(batch ConnectionBatch) {
	if batch.Reset {
		clear(t.active)
		clear(t.closed)
	}

	for _, event := range batch.Events {
		switch event.Type {
		case ConnectionNew:
			if event.Connection == nil || t.contains(event.ID) {
				continue
			}
			connection := cloneConnection(*event.Connection)
			connection.ID = event.ID
			if connection.ClosedAt.IsZero() {
				t.active[event.ID] = connection
			} else {
				connection.Uplink = 0
				connection.Downlink = 0
				t.closed[event.ID] = connection
			}
		case ConnectionUpdate:
			connection, ok := t.active[event.ID]
			if !ok {
				continue
			}
			connection.UplinkTotal += event.UplinkDelta
			connection.DownlinkTotal += event.DownlinkDelta
			connection.Uplink = event.UplinkDelta * int64(time.Second) / int64(t.interval)
			connection.Downlink = event.DownlinkDelta * int64(time.Second) / int64(t.interval)
			t.active[event.ID] = connection
		case ConnectionClosed:
			connection, ok := t.active[event.ID]
			if !ok {
				if event.Connection == nil || t.contains(event.ID) {
					continue
				}
				connection = cloneConnection(*event.Connection)
			}
			if event.Connection != nil {
				connection = cloneConnection(*event.Connection)
			}
			delete(t.active, event.ID)
			connection.ID = event.ID
			connection.Uplink = 0
			connection.Downlink = 0
			if !event.ClosedAt.IsZero() {
				connection.ClosedAt = event.ClosedAt
			}
			t.closed[event.ID] = connection
		}
	}
	t.trimClosed()
}

func (t *ConnectionTable) Active() []Connection {
	connections := mapConnections(t.active)
	sort.Slice(connections, func(i, j int) bool {
		if connections[i].CreatedAt.Equal(connections[j].CreatedAt) {
			return connections[i].ID < connections[j].ID
		}
		return connections[i].CreatedAt.Before(connections[j].CreatedAt)
	})
	return connections
}

func (t *ConnectionTable) Closed() []Connection {
	connections := mapConnections(t.closed)
	sort.Slice(connections, func(i, j int) bool {
		if connections[i].ClosedAt.Equal(connections[j].ClosedAt) {
			return connections[i].ID < connections[j].ID
		}
		return connections[i].ClosedAt.After(connections[j].ClosedAt)
	})
	return connections
}

func (t *ConnectionTable) All() []Connection {
	connections := t.Active()
	return append(connections, t.Closed()...)
}

func (t *ConnectionTable) Get(id string) (Connection, bool) {
	if connection, ok := t.active[id]; ok {
		return cloneConnection(connection), true
	}
	if connection, ok := t.closed[id]; ok {
		return cloneConnection(connection), true
	}
	return Connection{}, false
}

// Find returns the connection with exactly this id, or every connection whose
// id starts with it when id is at least four characters long. Callers treat
// zero results as not found and more than one as ambiguous.
func (t *ConnectionTable) Find(id string) []Connection {
	if connection, ok := t.Get(id); ok {
		return []Connection{connection}
	}
	if len(id) < 4 {
		return nil
	}
	var matches []Connection
	for _, connection := range t.All() {
		if strings.HasPrefix(connection.ID, id) {
			matches = append(matches, connection)
		}
	}
	return matches
}

func (t *ConnectionTable) contains(id string) bool {
	_, active := t.active[id]
	_, closed := t.closed[id]
	return active || closed
}

func (t *ConnectionTable) trimClosed() {
	closed := t.Closed()
	for index := t.closedLimit; index < len(closed); index++ {
		delete(t.closed, closed[index].ID)
	}
}

func mapConnections(source map[string]Connection) []Connection {
	connections := make([]Connection, 0, len(source))
	for _, connection := range source {
		connections = append(connections, cloneConnection(connection))
	}
	return connections
}

func cloneConnection(connection Connection) Connection {
	connection.Chain = append([]string(nil), connection.Chain...)
	if connection.Process != nil {
		process := *connection.Process
		process.PackageNames = append([]string(nil), process.PackageNames...)
		connection.Process = &process
	}
	return connection
}
