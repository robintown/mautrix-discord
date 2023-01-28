package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	log "maunium.net/go/maulogger/v2"

	"maunium.net/go/mautrix/crypto/attachment"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/util/dbutil"
)

type FileQuery struct {
	db  *Database
	log log.Logger
}

// language=postgresql
const (
	fileSelect = "SELECT url, encrypted, id, mxc, size, width, height, mime_type, decryption_info, timestamp FROM discord_file"
	fileInsert = `
		INSERT INTO discord_file (url, encrypted, id, mxc, size, width, height, mime_type, decryption_info, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
)

func (fq *FileQuery) New() *File {
	return &File{
		db:  fq.db,
		log: fq.log,
	}
}

func (fq *FileQuery) Get(url string, encrypted bool) *File {
	query := fileSelect + " WHERE url=$1 AND encrypted=$2"
	return fq.New().Scan(fq.db.QueryRow(query, url, encrypted))
}

type File struct {
	db  *Database
	log log.Logger

	URL       string
	Encrypted bool

	ID  string
	MXC id.ContentURI

	Size     int
	Width    int
	Height   int
	MimeType string

	DecryptionInfo *attachment.EncryptedFile

	Timestamp time.Time
}

func (f *File) Scan(row dbutil.Scannable) *File {
	var fileID, decryptionInfo sql.NullString
	var width, height sql.NullInt32
	var timestamp int64
	var mxc string
	err := row.Scan(&f.URL, &f.Encrypted, &fileID, &mxc, &f.Size, &width, &height, &f.MimeType, &decryptionInfo, &timestamp)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			f.log.Errorln("Database scan failed:", err)
			panic(err)
		}
		return nil
	}
	f.ID = fileID.String
	f.Timestamp = time.UnixMilli(timestamp)
	f.Width = int(width.Int32)
	f.Height = int(height.Int32)
	f.MXC, err = id.ParseContentURI(mxc)
	if err != nil {
		f.log.Errorfln("Failed to parse content URI %s: %v", mxc, err)
		panic(err)
	}
	if decryptionInfo.Valid {
		err = json.Unmarshal([]byte(decryptionInfo.String), &f.DecryptionInfo)
		if err != nil {
			f.log.Errorfln("Failed to unmarshal decryption info of %v: %v", f.MXC, err)
			panic(err)
		}
	}
	return f
}

func positiveIntToNullInt32(val int) (ptr sql.NullInt32) {
	if val > 0 {
		ptr.Valid = true
		ptr.Int32 = int32(val)
	}
	return
}

func (f *File) Insert(txn dbutil.Execable) {
	if txn == nil {
		txn = f.db
	}
	var err error
	var decryptionInfo []byte
	if f.DecryptionInfo != nil {
		decryptionInfo, err = json.Marshal(f.DecryptionInfo)
		if err != nil {
			f.log.Warnfln("Failed to marshal decryption info of %v: %v", f.MXC, err)
			panic(err)
		}
	}
	_, err = txn.Exec(fileInsert,
		f.URL, f.Encrypted, strPtr(f.ID), f.MXC.String(), f.Size,
		positiveIntToNullInt32(f.Width), positiveIntToNullInt32(f.Height), f.MimeType,
		string(decryptionInfo), f.Timestamp.UnixMilli(),
	)
	if err != nil {
		f.log.Warnfln("Failed to insert copied file %v: %v", f.MXC, err)
		panic(err)
	}
}

func (f *File) Delete() {
	_, err := f.db.Exec("DELETE FROM discord_file WHERE url=$1 AND encrypted=$2", f.URL, f.Encrypted)
	if err != nil {
		f.log.Warnfln("Failed to delete copied file %v: %v", f.MXC, err)
		panic(err)
	}
}
