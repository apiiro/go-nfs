package nfs

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/willscott/go-nfs-client/nfs/xdr"
)

type readDirPlusArgs struct {
	Handle      []byte
	Cookie      uint64
	CookieVerif uint64
	DirCount    uint32
	MaxCount    uint32
}

type readDirPlusEntity struct {
	FileID        uint64
	Name          []byte
	Cookie        uint64
	HasAttributes uint32
	Attributes    *FileAttribute
	HasHandle     uint32
	Handle        []byte
	Next          uint32
}

func onReadDirPlus(ctx context.Context, w *response, userHandle Handler) error {
	w.errorFmt = opAttrErrorFormatter
	obj := readDirPlusArgs{}
	if err := xdr.Read(w.req.Body, &obj); err != nil {
		return &NFSStatusError{NFSStatusInval, err}
	}

	fs, p, err := userHandle.FromHandle(obj.Handle)
	if err != nil {
		return &NFSStatusError{NFSStatusStale, err}
	}

	contents, err := fs.ReadDir(fs.Join(p...))
	if err != nil {
		return &NFSStatusError{NFSStatusNotDir, err}
	}

	if obj.DirCount < 1024 || obj.MaxCount < 4096 {
		return &NFSStatusError{NFSStatusTooSmall, nil}
	}

	entities := make([]readDirPlusEntity, 0)
	dirBytes := uint32(0)
	maxBytes := uint32(100) // conservative overhead measure

	isFirstRead := (obj.Cookie == 0)

	startIndex := 0
	if !isFirstRead {
		startIndex = int(obj.Cookie) - 2
	}
	i := startIndex
	for ; i < len(contents); i++ {
		content := contents[i]

		dirBytes += uint32(len(content.Name()) + 20)
		maxBytes += 512 // TODO: better estimation.

		if dirBytes > obj.DirCount || maxBytes > obj.MaxCount || len(entities) > userHandle.HandleLimit()/2 {
			break
		}

		handle := userHandle.ToHandle(fs, append(p, content.Name()))
		attrs := ToFileAttribute(content)
		attrs.Fileid = binary.BigEndian.Uint64(handle[0:8])
		entities = append(entities, readDirPlusEntity{
			FileID:        binary.BigEndian.Uint64(handle[0:8]),
			Name:          []byte(content.Name()),
			Cookie:        uint64(i + 3),
			HasAttributes: 1,
			Attributes:    attrs,
			HasHandle:     1,
			Handle:        handle,
			Next:          1,
		})
	}

	writer := bytes.NewBuffer([]byte{})
	if err := xdr.Write(writer, uint32(NFSStatusOk)); err != nil {
		return &NFSStatusError{NFSStatusServerFault, err}
	}
	if err := WritePostOpAttrs(writer, tryStat(fs, p)); err != nil {
		return &NFSStatusError{NFSStatusServerFault, err}
	}

	cookieVerifcation := [8]byte{}
	if err := xdr.Write(writer, cookieVerifcation); err != nil {
		return &NFSStatusError{NFSStatusServerFault, err}
	}

	if isFirstRead {
		// prefix the special "." and ".." entries.
		if err := xdr.Write(writer, uint32(1)); err != nil { //next
			return &NFSStatusError{NFSStatusServerFault, err}
		}
		if err := xdr.Write(writer, uint64(binary.BigEndian.Uint64(obj.Handle[0:8]))); err != nil { //fileID
			return &NFSStatusError{NFSStatusServerFault, err}
		}
		if err := xdr.Write(writer, []byte(".")); err != nil { // name
			return &NFSStatusError{NFSStatusServerFault, err}
		}
		if err := xdr.Write(writer, uint64(1)); err != nil { // cookie
			return &NFSStatusError{NFSStatusServerFault, err}
		}
		if err := xdr.Write(writer, uint32(0)); err != nil { // hasAttribute
			return &NFSStatusError{NFSStatusServerFault, err}
		}
		if err := xdr.Write(writer, uint32(0)); err != nil { // hasHandle
			return &NFSStatusError{NFSStatusServerFault, err}
		}
		if err := xdr.Write(writer, uint32(1)); err != nil { // next
			return &NFSStatusError{NFSStatusServerFault, err}
		}
		if len(p) > 0 {
			ph := userHandle.ToHandle(fs, p[0:len(p)-1])
			if err := xdr.Write(writer, uint64(binary.BigEndian.Uint64(ph[0:8]))); err != nil { //fileID
				return &NFSStatusError{NFSStatusServerFault, err}
			}
		} else {
			if err := xdr.Write(writer, uint64(0)); err != nil { //fileID
				return &NFSStatusError{NFSStatusServerFault, err}
			}
		}
		if err := xdr.Write(writer, []byte("..")); err != nil { //name
			return &NFSStatusError{NFSStatusServerFault, err}
		}
		if err := xdr.Write(writer, uint64(2)); err != nil { // cookie
			return &NFSStatusError{NFSStatusServerFault, err}
		}
		if err := xdr.Write(writer, uint32(0)); err != nil { // hasAttribute
			return &NFSStatusError{NFSStatusServerFault, err}
		}
		if err := xdr.Write(writer, uint32(0)); err != nil { // hasHandle
			return &NFSStatusError{NFSStatusServerFault, err}
		}
	}
	if len(entities) > 0 || isFirstRead {
		if err := xdr.Write(writer, uint32(1)); err != nil { // next
			return &NFSStatusError{NFSStatusServerFault, err}
		}
	}
	if len(entities) > 0 {
		entities[len(entities)-1].Next = 0
		// the 'yes there is a 1st entity' bool
	}
	for _, e := range entities {
		if err := xdr.Write(writer, e); err != nil {
			return &NFSStatusError{NFSStatusServerFault, err}
		}
	}
	isEof := uint32(0)
	if len(entities) == 0 || i == len(contents) {
		isEof = 1
	}
	if err := xdr.Write(writer, isEof); err != nil {
		return &NFSStatusError{NFSStatusServerFault, err}
	}
	// TODO: track writer size at this point to validate maxcount estimation and stop early if needed.

	if err := w.Write(writer.Bytes()); err != nil {
		return &NFSStatusError{NFSStatusServerFault, err}
	}
	return nil
}
