package standalone_storage

import (
	"github.com/Connor1996/badger"
	"github.com/pingcap-incubator/tinykv/kv/config"
	"github.com/pingcap-incubator/tinykv/kv/storage"
	"github.com/pingcap-incubator/tinykv/kv/util/engine_util"
	"github.com/pingcap-incubator/tinykv/proto/pkg/kvrpcpb"
)

// StandAloneStorage is an implementation of `Storage` for a single-node TinyKV instance. It does not
// communicate with other nodes and all data is stored locally.
type StandAloneStorage struct {
	// Your Data Here (1).
	Engines *engine_util.Engines
}

func NewStandAloneStorage(conf *config.Config) *StandAloneStorage {
	// Your Code Here (1).
	e := engine_util.NewEngines(engine_util.CreateDB(conf.DBPath, false), nil, conf.DBPath, "")
	return &StandAloneStorage{e}
}

func (s *StandAloneStorage) Start() error {
	// Your Code Here (1).
	return nil
}

func (s *StandAloneStorage) Stop() error {
	// Your Code Here (1).
	return nil
}

func (s *StandAloneStorage) Reader(ctx *kvrpcpb.Context) (storage.StorageReader, error) {
	// Your Code Here (1).
	return &AloneStorageReader{s.Engines.Kv, s.Engines.Kv.NewTransaction(false)}, nil
}

func (s *StandAloneStorage) Write(ctx *kvrpcpb.Context, batch []storage.Modify) error {
	// Your Code Here (1).
	writeBatch := engine_util.WriteBatch{}
	for _, b := range batch {
		writeBatch.SetCF(b.Cf(), b.Key(), b.Value())
	}
	return s.Engines.WriteKV(&writeBatch)
}

type AloneStorageReader struct {
	Kv  *badger.DB
	Txn *badger.Txn
}

func (alone *AloneStorageReader) GetCF(cf string, key []byte) ([]byte, error) {
	value, err := engine_util.GetCF(alone.Kv, cf, key)
	if value == nil {
		return nil, nil
	}
	return value, err
}

func (alone *AloneStorageReader) IterCF(cf string) engine_util.DBIterator {
	return engine_util.NewCFIterator(cf, alone.Txn)
}

func (alone *AloneStorageReader) Close() {
	alone.Kv.Close()
	alone.Txn.Discard()
}
