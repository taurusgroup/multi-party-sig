package pb

import "github.com/taurusgroup/cmp-ecdsa/pkg/party"

func (x *Message) GetFromID() party.ID {
	return party.ID(x.From)
}

func (x *Message) GetToID() party.ID {
	return party.ID(x.To)
}

func (x *Message) IsValid() bool {
	switch x.Type {
	// refresh_old
	case MessageType_TypeRefresh1:
		return x.GetRefresh1() != nil
	case MessageType_TypeRefresh2:
		return x.GetRefresh2() != nil
	case MessageType_TypeRefresh3:
		return x.GetRefresh3() != nil
	case MessageType_TypeRefresh4:
		return x.GetRefresh4() != nil
	// sign
	case MessageType_TypeSign1:
		return x.GetSign1() != nil
	case MessageType_TypeSign2:
		return x.GetSign2() != nil
	case MessageType_TypeSign3:
		return x.GetSign3() != nil
	case MessageType_TypeSign4:
		return x.GetSign4() != nil
	// sign abort
	case MessageType_TypeAbort1:
		return x.GetAbort1() != nil
	case MessageType_TypeAbort2:
		return x.GetAbort2() != nil
	// old keygen
	case MessageType_TypeKeygenOld1:
		return x.GetKeygenOld1() != nil
	case MessageType_TypeKeygenOld2:
		return x.GetKeygenOld2() != nil
	case MessageType_TypeKeygenOld3:
		return x.GetKeygenOld3() != nil
	// old refresh_old
	case MessageType_TypeRefreshOld1:
		return x.GetRefreshOld1() != nil
	case MessageType_TypeRefreshOld2:
		return x.GetRefreshOld2() != nil
	case MessageType_TypeRefreshOld3:
		return x.GetRefreshOld3() != nil
	}
	return false
}
