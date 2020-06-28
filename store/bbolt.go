package store

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gloworm-vision/gloworm-app/hardware"
	"github.com/gloworm-vision/gloworm-app/pipeline"
	"go.etcd.io/bbolt"
)

type BBolt struct {
	db *bbolt.DB
}

const (
	bboltGlowormBucket        = "gloworm"
	bboltPipelineConfigBucket = "pipeline-configs" // child of gloworm

	// gloworm keys
	bboltHardwareKey              = "hardware"
	bboltDefaultPipelineConfigKey = "default-pipeline-config"
)

// OpenBBolt opens a BBoltDB database at the given path and creates the needed buckets
// if they don't exist.
func OpenBBolt(path string, mode os.FileMode, options *bbolt.Options) (Store, error) {
	db, err := bbolt.Open(path, mode, options)
	if err != nil {
		return nil, fmt.Errorf("unable to open bbolt db: %w", err)
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		glowormBucket, err := tx.CreateBucketIfNotExists([]byte(bboltGlowormBucket))
		if err != nil {
			return fmt.Errorf("unable to create bucket %q: %w", bboltGlowormBucket, err)
		}

		_, err = glowormBucket.CreateBucketIfNotExists([]byte(bboltPipelineConfigBucket))
		if err != nil {
			return fmt.Errorf("unable to create bucket %q: %w", bboltPipelineConfigBucket, err)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create bbolt buckets: %w", err)
	}

	return &BBolt{
		db: db,
	}, nil
}

func (b *BBolt) Close() error {
	return b.Close()
}

func (b *BBolt) PipelineConfig(name string) (pipeline.Config, error) {
	var p pipeline.Config
	err := b.db.View(func(tx *bbolt.Tx) error {
		glowormBucket := tx.Bucket([]byte(bboltGlowormBucket))
		configBucket := glowormBucket.Bucket([]byte(bboltPipelineConfigBucket))

		pipelineJSON := configBucket.Get([]byte(name))
		if pipelineJSON == nil {
			return fmt.Errorf("pipeline config does not exist")
		}

		if err := json.Unmarshal(pipelineJSON, &p); err != nil {
			return fmt.Errorf("unable to unmarshal pipeline config JSON: %w", err)
		}

		return nil
	})
	if err != nil {
		return p, fmt.Errorf("unable to get pipeline config %q: %w", name, err)
	}

	return p, nil
}

func (b *BBolt) ListPipelineConfigs() ([]string, error) {
	names := make([]string, 0)

	err := b.db.View(func(tx *bbolt.Tx) error {
		glowormBucket := tx.Bucket([]byte(bboltGlowormBucket))
		configBucket := glowormBucket.Bucket([]byte(bboltPipelineConfigBucket))

		err := configBucket.ForEach(func(k, _ []byte) error {
			names = append(names, string(k))
			return nil
		})
		if err != nil {
			return fmt.Errorf("unable to iterate over config bucket: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to list pipeline configs: %w", err)
	}

	return names, nil
}

func (b *BBolt) PutPipelineConfig(name string, p pipeline.Config) error {
	err := b.db.Update(func(tx *bbolt.Tx) error {
		pipelineJSON, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("unable to marshal pipeline config: %w", err)
		}

		glowormBucket := tx.Bucket([]byte(bboltGlowormBucket))
		configBucket := glowormBucket.Bucket([]byte(bboltPipelineConfigBucket))
		if err := configBucket.Put([]byte(name), pipelineJSON); err != nil {
			return fmt.Errorf("unable to put pipeline config %q: %w", name, err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("unable to update pipeline config: %w", err)
	}

	return nil
}

func (b *BBolt) DefaultPipelineConfig() (string, error) {
	var def string

	err := b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bboltGlowormBucket))
		def = string(bucket.Get([]byte(bboltDefaultPipelineConfigKey)))
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("unable to get default pipeline config: %w", err)
	}

	return def, nil
}

func (b *BBolt) PutDefaultPipelineConfig(def string) error {
	err := b.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bboltGlowormBucket))
		bucket.Put([]byte(bboltDefaultPipelineConfigKey), []byte(def))
		return nil
	})
	if err != nil {
		return fmt.Errorf("unable to put default pipeline config: %w", err)
	}

	return nil
}

func (b *BBolt) HardwareConfig() (hardware.Config, error) {
	var h hardware.Config
	err := b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bboltGlowormBucket))
		hardwareJSON := bucket.Get([]byte(bboltHardwareKey))
		if hardwareJSON == nil {
			return fmt.Errorf("hardware config does not exist")
		}

		if err := json.Unmarshal(hardwareJSON, &h); err != nil {
			return fmt.Errorf("unable to unmarshal hardware config JSON: %w", err)
		}

		return nil
	})
	if err != nil {
		return h, fmt.Errorf("unable to get hardware config: %w", err)
	}

	return h, nil
}

func (b *BBolt) PutHardwareConfig(p hardware.Config) error {
	err := b.db.Update(func(tx *bbolt.Tx) error {
		hardwareJSON, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("unable to marshal hardware config: %w", err)
		}

		bucket := tx.Bucket([]byte(bboltGlowormBucket))
		if err := bucket.Put([]byte(bboltHardwareKey), hardwareJSON); err != nil {
			return fmt.Errorf("unable to put hardware config: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("unable to update hardware config: %w", err)
	}

	return nil
}
