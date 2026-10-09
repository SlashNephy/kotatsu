package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	envPrefix  = "KOTATSU_"
	fileSuffix = "_FILE"
)

// LookupEnv は環境変数を引く関数。os.LookupEnv と同じ形をとる。
type LookupEnv func(key string) (string, bool)

// Load は設定を読み込む。後のものほど優先する。
//
//  1. 既定値
//  2. path の YAML (path が空なら読まない)
//  3. KOTATSU_ で始まる環境変数
//  4. 名前が _FILE で終わる環境変数で指定したファイル (Secret 型の項目だけ)
//
// 同じ項目に 3 と 4 の両方が与えられたときは、どちらを使うべきか判断できないのでエラーにする。
// 返すエラーには項目名を含め、秘密情報の値は含めない。
func Load(path string, lookupEnv LookupEnv) (*Config, error) {
	cfg := Default()

	if path != "" {
		if err := loadYAML(path, cfg); err != nil {
			return nil, err
		}
	}

	if err := applyEnv(reflect.ValueOf(cfg).Elem(), []string{}, lookupEnv); err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadYAML(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read the config file: %w", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	// 綴りを間違えたキーが黙って無視されないよう、未知のキーを拒否する。
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to parse the config file %s: %w", path, err)
	}

	return nil
}

// applyEnv は構造体の各項目を、対応する環境変数で上書きし、必須の項目が埋まっているかを確かめる。
func applyEnv(v reflect.Value, keys []string, lookupEnv LookupEnv) error {
	errs := []error{}
	for i := range v.NumField() {
		field := v.Type().Field(i)
		key, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		if key == "" || key == "-" {
			continue
		}

		fieldKeys := slices.Concat(keys, []string{key})
		value := v.Field(i)
		if value.Kind() == reflect.Struct {
			if err := applyEnv(value, fieldKeys, lookupEnv); err != nil {
				errs = append(errs, err)
			}

			continue
		}

		path := strings.Join(fieldKeys, ".")
		if err := applyEnvToField(value, fieldKeys, lookupEnv); err != nil {
			errs = append(errs, fieldError(path, err))

			continue
		}

		if field.Tag.Get("required") == "true" && value.IsZero() {
			errs = append(errs, fieldError(path, errors.New("is required")))
		}
	}

	return errors.Join(errs...)
}

func applyEnvToField(value reflect.Value, keys []string, lookupEnv LookupEnv) error {
	name := envName(keys)
	raw, hasValue := lookupEnv(name)

	if value.Type() == reflect.TypeFor[Secret]() {
		filePath, hasFile := lookupEnv(name + fileSuffix)
		switch {
		case hasValue && hasFile:
			return fmt.Errorf("both %s and %s are set; set only one of them", name, name+fileSuffix)
		case hasFile:
			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read the file given by %s: %w", name+fileSuffix, err)
			}
			// echo や多くのエディタはファイルの末尾に改行を付けるので、取り除く。
			raw, hasValue = strings.TrimRight(string(data), "\r\n"), true
		}
	}

	// 対応していない型の項目を Config に足したときに黙って無視しないよう、環境変数の有無にかかわらず型を確かめる。
	switch ptr := value.Addr().Interface().(type) {
	case *string:
		if hasValue {
			*ptr = raw
		}
	case *Secret:
		if hasValue {
			*ptr = Secret(raw)
		}
	case *int:
		if hasValue {
			n, err := strconv.Atoi(raw)
			if err != nil {
				return errors.New("must be an integer")
			}
			*ptr = n
		}
	case *bool:
		if hasValue {
			b, err := strconv.ParseBool(raw)
			if err != nil {
				return errors.New("must be a boolean")
			}
			*ptr = b
		}
	default:
		return fmt.Errorf("unsupported field type %s", value.Type())
	}

	return nil
}

// envName は YAML のキーの並びから環境変数の名前を作る (database, url → KOTATSU_DATABASE_URL)。
func envName(keys []string) string {
	return envPrefix + strings.ToUpper(strings.Join(keys, "_"))
}

func fieldError(path string, err error) error {
	return fmt.Errorf("invalid config %s (%s): %w", path, envName(strings.Split(path, ".")), err)
}
