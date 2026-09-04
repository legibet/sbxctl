package sbx

import (
	"time"

	"github.com/legibet/sbxctl/internal/daemon"
)

func unixSeconds(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(value, 0)
}

func unixMilliseconds(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.UnixMilli(value)
}

func convertStatus(value *daemon.Status) Status {
	return Status{
		Memory:           value.Memory,
		Goroutines:       int(value.Goroutines),
		ConnectionsIn:    int(value.ConnectionsIn),
		ConnectionsOut:   int(value.ConnectionsOut),
		TrafficAvailable: value.TrafficAvailable,
		Uplink:           value.Uplink,
		Downlink:         value.Downlink,
		UplinkTotal:      value.UplinkTotal,
		DownlinkTotal:    value.DownlinkTotal,
	}
}

var serviceStates = [...]string{"idle", "starting", "started", "stopping", "fatal"}

func convertServiceStatus(value *daemon.ServiceStatus) ServiceStatus {
	state := "idle"
	if index := int(value.Status); index >= 0 && index < len(serviceStates) {
		state = serviceStates[index]
	}
	return ServiceStatus{State: state, Error: value.ErrorMessage}
}

func convertOutbounds(values []*daemon.GroupItem) []Outbound {
	outbounds := make([]Outbound, 0, len(values))
	for _, value := range values {
		outbounds = append(outbounds, convertOutbound(value))
	}
	return outbounds
}

func convertOutbound(value *daemon.GroupItem) Outbound {
	return Outbound{
		Tag:      value.Tag,
		Type:     value.Type,
		Delay:    int(value.UrlTestDelay),
		TestedAt: unixSeconds(value.UrlTestTime),
	}
}

func convertGroups(values []*daemon.Group) []Group {
	groups := make([]Group, 0, len(values))
	for _, value := range values {
		groups = append(groups, Group{
			Tag:        value.Tag,
			Type:       value.Type,
			Selectable: value.Selectable,
			Selected:   value.Selected,
			Items:      convertOutbounds(value.Items),
		})
	}
	return groups
}

func convertProcess(value *daemon.ProcessInfo) *Process {
	if value == nil {
		return nil
	}
	packages := make([]string, 0, len(value.PackageNames))
	packages = append(packages, value.PackageNames...)
	return &Process{
		PID:          value.ProcessId,
		UserID:       int(value.UserId),
		UserName:     value.UserName,
		Path:         value.ProcessPath,
		PackageNames: packages,
	}
}

func convertConnection(value *daemon.Connection) *Connection {
	if value == nil {
		return nil
	}
	chain := make([]string, 0, len(value.ChainList))
	chain = append(chain, value.ChainList...)
	return &Connection{
		ID:           value.Id,
		Inbound:      value.Inbound,
		InboundType:  value.InboundType,
		IPVersion:    int(value.IpVersion),
		Network:      value.Network,
		Source:       value.Source,
		Destination:  value.Destination,
		Domain:       value.Domain,
		Protocol:     value.Protocol,
		User:         value.User,
		FromOutbound: value.FromOutbound,
		CreatedAt:    unixMilliseconds(value.CreatedAt),
		ClosedAt:     unixMilliseconds(value.ClosedAt),
		// Uplink and Downlink rates are never populated by the server;
		// ConnectionTable derives them from update deltas.
		UplinkTotal:   value.UplinkTotal,
		DownlinkTotal: value.DownlinkTotal,
		Rule:          value.Rule,
		Outbound:      value.Outbound,
		OutboundType:  value.OutboundType,
		Chain:         chain,
		Process:       convertProcess(value.ProcessInfo),
	}
}

func convertConnectionBatch(value *daemon.ConnectionEvents) ConnectionBatch {
	events := make([]ConnectionEvent, 0, len(value.Events))
	for _, event := range value.Events {
		events = append(events, ConnectionEvent{
			Type:          ConnectionEventType(event.Type),
			ID:            event.Id,
			Connection:    convertConnection(event.Connection),
			UplinkDelta:   event.UplinkDelta,
			DownlinkDelta: event.DownlinkDelta,
			ClosedAt:      unixMilliseconds(event.ClosedAt),
		})
	}
	return ConnectionBatch{Reset: value.Reset_, Events: events}
}

func convertLogBatch(value *daemon.Log) LogBatch {
	entries := make([]LogEntry, 0, len(value.Messages))
	for _, message := range value.Messages {
		entries = append(entries, LogEntry{Level: LogLevel(message.Level), Message: message.Message})
	}
	return LogBatch{Reset: value.Reset_, Entries: entries}
}
