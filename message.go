package pdu // import "github.com/webdeskltd/pdu"

//import "github.com/webdeskltd/debug"
//import "github.com/webdeskltd/log"
import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/webdeskltd/pdu/encoders"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// Complete return true if decoding of message completed
func (msg *message) Complete() bool { return msg.End }

// Error Last error
func (msg *message) Error() error { return msg.Err }

func (msg *message) findCommand(src *bytes.Buffer) {
	var idx []int

	idx = rexDataWithCommand.FindSubmatchIndex(src.Bytes())
	if idx == nil {
		return
	}
	msg.Cmd = string(src.Bytes()[idx[2]:idx[3]])
	msg.CmdStat, msg.Err = strconv.ParseInt(string(src.Bytes()[idx[4]:idx[5]]), 0, 64)
	if msg.Err != nil {
		msg.CmdStat = 0
	}
	msg.CmdAlpha = string(src.Bytes()[idx[6]:idx[7]])
	msg.CmdLength, msg.Err = strconv.ParseInt(string(src.Bytes()[idx[8]:idx[9]]), 0, 64)
	if msg.Err != nil {
		msg.CmdLength = 0
	}
}

func (msg *message) findPDU(src *bytes.Buffer) {
	var idx []int
	idx = rexDataWithoutCommand.FindSubmatchIndex(src.Bytes())
	if idx == nil {
		return
	}
	msg.DataSource, msg.Err = hex.DecodeString(string(src.Bytes()[idx[2]:idx[3]]))
}

// Load Service Centre Address
func (msg *message) loadTpSca() {
	var tmp byte
	var pe int
	var buf []byte
	tmp = msg.DataSource[msg.Lp]
	msg.Lp++
	msg.TpScaLen = tmp
	if msg.TpScaLen == 0 {
		return
	}
	msg.TpScaTypeSource = msg.DataSource[msg.Lp]
	msg.Lp++

	pe = msg.Lp + int(msg.TpScaLen)
	buf = msg.DataSource[msg.Lp:pe]
	msg.TpScaNumber = encoders.NewSemiOctet().DecodeAddress(buf)
	msg.TpScaNumericPlan, msg.TpScaType = decodeNumberType(int(msg.TpScaTypeSource))
	msg.Lp += int(msg.TpScaLen) - 1
	return
}

// Load Originating Address
func (msg *message) loadTpOa() {
	var tmp byte
	var pe int
	var buf []byte
	var dl float64
	tmp = msg.DataSource[msg.Lp]
	msg.Lp++
	msg.TpOaLen = tmp
	if msg.TpOaLen == 0 {
		return
	}
	msg.TpOaTypeSource = msg.DataSource[msg.Lp]
	msg.Lp++
	dl = float64(msg.TpOaLen) * 4 / 8
	if float64(int64(dl)) < dl {
		dl = float64(int64(dl)) + 1
	}
	pe = msg.Lp + int(dl)
	buf = msg.DataSource[msg.Lp:pe]
	msg.TpOaNumber = encoders.NewSemiOctet().DecodeAddress(buf)
	msg.TpOaNumericPlan, msg.TpOaType = decodeNumberType(int(msg.TpOaTypeSource))
	msg.Lp += int(dl)
	return
}

// Load Protocol identifier (TP-PID)
func (msg *message) loadPid() {
	msg.Pid = msg.DataSource[msg.Lp]
	msg.Lp++
}

// Load Message Type indicator
func (msg *message) loadMti() {
	var tmp byte
	msg.MtiSource = msg.DataSource[msg.Lp]
	msg.Lp++
	tmp = msg.MtiSource
	if tmp&(1<<7) > 0 {
		msg.MtiReplyPath = true
	}
	if tmp&(1<<6) > 0 {
		msg.MtiUdhiFound = true
	}
	if tmp&(1<<5) > 0 {
		msg.MtiStatusReport = true
	}
	if tmp&(1<<2) > 0 {
		msg.MtiNoMoreMessageToSend = true
	}
	tmp &= 0x3
	switch tmp {
	case 0:
		msg.MtiSmsType = TypeSmsDeliver
	case 1:
		msg.MtiSmsType = TypeSmsSubmitReport
	case 2:
		msg.MtiSmsType = TypeSmsStatusReport
	case 3:
		msg.MtiSmsType = TypeSmsReserved
	}
}

// Load Data coding scheme TP-SCTS
func (msg *message) loadDsc() {
	msg.DcsSource = msg.DataSource[msg.Lp]
	msg.Lp++
	if msg.DcsSource&(1<<4) > 0 {
		msg.DscFlash = true
	}
	if msg.DcsSource&(1<<3) > 0 {
		msg.DscUSC2 = true
	}
}

// Load User Data (TP-UD)
func (msg *message) loadUd() {
	var pe int
	msg.SmsDataSourceLength = msg.DataSource[msg.Lp]
	msg.Lp++

	switch msg.DscUSC2 {
	case true:
		// UTF-16
		pe = int(msg.SmsDataSourceLength) + msg.Lp
		if pe > len(msg.DataSource) {
			msg.Err = fmt.Errorf("Message is corrupted. User data is too short")
			msg.End = true
			return
		}
		msg.SmsDataSource = msg.DataSource[msg.Lp:pe]
		msg.Lp += int(msg.SmsDataSourceLength)
	case false:
		// 7-bit piece of shit
		var dl = float64(msg.SmsDataSourceLength) * 7 / 8
		if float64(int64(dl)) < dl {
			dl = float64(int64(dl)) + 1
		}
		pe = msg.Lp + int(dl)
		msg.SmsDataSource = msg.DataSource[msg.Lp:pe]
		msg.Lp += int(dl)
	}

}

// Load The service centre time stamp (TP-SCTS)
func (msg *message) loadScts() {
	var buf []byte
	var str string
	var y, m, d, H, M, S, t int
	buf = msg.DataSource[msg.Lp : msg.Lp+7]
	msg.Lp += 7
	str = encoders.NewSemiOctet().DecodeAddress(buf)
	y, msg.Err = strconv.Atoi(str[0:2])
	m, msg.Err = strconv.Atoi(str[2:4])
	d, msg.Err = strconv.Atoi(str[4:6])
	H, msg.Err = strconv.Atoi(str[6:8])
	M, msg.Err = strconv.Atoi(str[8:10])
	S, msg.Err = strconv.Atoi(str[10:12])
	t, msg.Err = strconv.Atoi(str[12:14])
	msg.ServiceCentreTimeStamp = time.Date(y+2000, time.Month(m), d, H, M, S, 0, time.UTC).Add(time.Hour * time.Duration(t)).In(time.Local)
}

// Load user data header information from user data
func (msg *message) loadUdhi() {
	var pe, lp int
	if !msg.MtiUdhiFound {
		return
	}
	msg.UdhiLength = msg.SmsDataSource[lp]
	lp++
	pe = lp + int(msg.UdhiLength)
	msg.UdhiSource = msg.SmsDataSource[lp:pe]
	lp += int(msg.UdhiLength)
	// Cut header from user data
	msg.SmsDataSource = msg.SmsDataSource[lp:]
	msg.SmsDataSourceLength -= (msg.UdhiLength + uint8(1))
	// Decode header
	lp = 0
	msg.UdhiIei = msg.UdhiSource[lp]
	lp++
	msg.UdhiIedl = msg.UdhiSource[lp]
	lp++
	msg.UdhiIedID = uint16(msg.UdhiSource[lp])
	lp++
	if msg.UdhiIedl == 4 {
		msg.UdhiIedID = msg.UdhiIedID << 8
		msg.UdhiIedID = msg.UdhiIedID & uint16(msg.UdhiSource[lp])
		lp++
	}
	msg.UdhiNumberParts = msg.UdhiSource[lp]
	lp++
	msg.UdhiSequenceID = msg.UdhiSource[lp]
}

// Decode loaded user data
func (msg *message) decodeUD() {
	msg.loadUdhi()
	if msg.DscUSC2 {
		var buf []byte
		var enc = unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)
		var tnsf = enc.NewDecoder()
		buf, msg.SmsDataLength, msg.Err = transform.Bytes(tnsf, msg.SmsDataSource)
		if msg.SmsDataLength > 0 && msg.Err == nil {
			msg.SmsData = string(buf)
		} else {
			msg.SmsDataLength = 0
		}
	} else {
		msg.SmsData, msg.Err = encoders.New7Bit().Decode(msg.SmsDataSource)
	}
}

// Scan Source data scanning
func (msg *message) Scan(src *bytes.Buffer) {
	msg.findCommand(src)
	msg.findPDU(src)
	// Error, PDU data is empty
	if len(msg.DataSource) == 0 {
		msg.Err = ErrIncorrectPDUdata
		msg.End = true
		return
	}
	msg.loadTpSca()
	msg.loadMti()
	msg.loadTpOa()
	msg.loadPid()
	msg.loadDsc()
	msg.loadScts()
	msg.loadUd()
	msg.decodeUD()

	// Clean
	msg.DataSource = []byte{}
	msg.SmsDataSource = []byte{}
	msg.UdhiSource = []byte{}
	if msg.End {
		return
	}

	// No header no parts
	if !msg.MtiUdhiFound {
		msg.End = true
		return
	}

	// Number of parts
	if msg.UdhiNumberParts == 1 {
		msg.End = true
		return
	}
}

// Direction Message direction
func (msg *message) Direction() MessageDirection { return msg.Dir }

// Create Date and time begin of decode message
func (msg *message) Create() time.Time { return msg.CreateTime }

// Command If the message contained a command, this function returns it
func (msg *message) Command() string { return msg.Cmd }

// ServiceCentreAddress Return service centre address
func (msg *message) ServiceCentreAddress() string { return msg.TpScaNumber }

// ServiceCentreType Return service centre address type
func (msg *message) ServiceCentreType() NumberType { return msg.TpScaType }

// ServiceCentreNumericPlan Return service centre numbering plan identifier
func (msg *message) ServiceCentreNumericPlan() NumberNumericPlan { return msg.TpScaNumericPlan }

// ServiceCentreTime Service centre time stamp
func (msg *message) ServiceCentreTime() time.Time { return msg.ServiceCentreTimeStamp }

// Type Return message type indicator (MTI)
func (msg *message) Type() SmsType { return msg.MtiSmsType }

// IsStatusReport Status report indication (TP-SRI)
func (msg *message) IsStatusReport() bool { return msg.MtiStatusReport }

// Reply path (TP-RP) if =true-A response is requested
func (msg *message) IsReplyPath() bool { return msg.MtiReplyPath }

// OriginatingAddress Originating address
func (msg *message) OriginatingAddress() string { return msg.TpOaNumber }

// OriginatingAddressType Originating address type
func (msg *message) OriginatingAddressType() NumberType { return msg.TpOaType }

// OriginatingAddressNumericPlan Originating address numbering plan identifier
func (msg *message) OriginatingAddressNumericPlan() NumberNumericPlan { return msg.TpOaNumericPlan }

// ProtocolIdentifier Protocol identifier
func (msg *message) ProtocolIdentifier() uint8 { return msg.Pid }

// IsFlash Message is flash
func (msg *message) IsFlash() bool { return msg.DscFlash }

// IsEncode7Bit Message encoded as 7bit asci
func (msg *message) IsEncode7Bit() bool { return !msg.DscUSC2 }

// IsEncodeUSC2 Message encoded as UCS2 (UTF-16)
func (msg *message) IsEncodeUSC2() bool { return msg.DscUSC2 }

// Data Decoded message data
func (msg *message) Data() string { return msg.SmsData }

// DataParts The number of SMS (parts)
func (msg *message) DataParts() (ret int) {
	ret = 1
	if msg.MtiUdhiFound {
		ret = int(msg.UdhiNumberParts)
	}
	return
}