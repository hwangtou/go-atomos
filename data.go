package atomos

import (
	"fmt"
	"log"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/syndtr/goleveldb/leveldb"
)

type dataStorage struct {
	db *leveldb.DB
}

func (d *dataStorage) open(filepath string) (err error) {
	d.db, err = leveldb.OpenFile(filepath, nil)
	return err
}

func (d *dataStorage) atomKey(t, n string) []byte {
	return []byte(fmt.Sprintf("aIns:%s:%s", t, n))
}

func (d *dataStorage) loadAtom(t, n string) (*AtomData, error) {
	buf, err := d.db.Get(d.atomKey(t, n), nil)
	if err != nil {
		if err != leveldb.ErrNotFound {
			return nil, err
		}
		return &AtomData{
			LogId: 0,
		}, nil
	}
	a := &AtomData{}
	if err = proto.Unmarshal(buf, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (d *dataStorage) saveAtom(t, n string, data *AtomData) error {
	buf, err := proto.Marshal(data)
	if err != nil {
		return err
	}
	return d.db.Put(d.atomKey(t, n), buf, nil)
}

func (d *dataStorage) logAtom(aType, aName string, id uint64, level LogLevel, msg string) {
	// Get key & set key
	timeString := time.Now().UTC().Format("2000-01-01T00:00:00.000Z")
	key := []byte(fmt.Sprintf("logAtom:%s:%s:%v", aType, aName, id))
	value := fmt.Sprintf("%s atomos:[%s:%s] %s %s", timeString, aType, aName, level, msg)
	print(value)
	if err := d.db.Put(key, []byte(value), nil); err != nil {
		log.Println(err)
	}
}
