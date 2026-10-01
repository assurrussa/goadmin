package browserstate

import (
	"context"
	"errors"
	"time"

	"github.com/assurrussa/goauth"
	redis "github.com/redis/go-redis/v9"
)

type Redis struct {
	conn  redis.UniversalClient
	codec codec
}

func NewRedis(conn redis.UniversalClient, keys goauth.KeyRing) *Redis {
	return &Redis{conn: conn, codec: codec{keys: keys}}
}
func redisKey(id string) string { return "goadmin:auth-state:" + id }
func (s *Redis) Load(ctx context.Context, id string) (Record, bool, error) {
	raw, err := s.conn.Get(ctx, redisKey(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	r, err := s.codec.decode(id, raw)
	if err != nil {
		return Record{}, false, err
	}
	return r, true, nil
}

func (s *Redis) Create(ctx context.Context, id string, r Record, ttl time.Duration) error {
	r.Version = 1
	r.Owner = ""
	raw, err := s.codec.encode(id, r)
	if err != nil {
		return err
	}
	if ttl <= 0 {
		return errors.New("admin auth state lifetime must be positive")
	}
	ok, err := s.conn.SetNX(ctx, redisKey(id), raw, ttl).Result()
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("admin auth state already exists")
	}
	return nil
}

const claimScript = `
local raw=redis.call('GET',KEYS[1])
if not raw then return 0 end
local r=cjson.decode(raw)
if r.version~=tonumber(ARGV[1]) or r.owner~='' then return 0 end
r.owner=ARGV[2]
redis.call('SET',KEYS[1],cjson.encode(r),'KEEPTTL')
return 1`

const completeScript = `
local raw=redis.call('GET',KEYS[1])
if not raw then return 0 end
local old=cjson.decode(raw)
if old.version~=tonumber(ARGV[1]) or old.owner~=ARGV[2] then return 0 end
redis.call('SET',KEYS[1],ARGV[3],'KEEPTTL')
return 1`

func (s *Redis) Claim(ctx context.Context, id string, v int64, owner string) (bool, error) {
	n, err := s.conn.Eval(ctx, claimScript, []string{redisKey(id)}, v, owner).Int()
	return n == 1, err
}

func (s *Redis) Complete(ctx context.Context, id string, v int64, owner string, r Record) (bool, error) {
	r.Version = v + 1
	r.Owner = ""
	raw, err := s.codec.encode(id, r)
	if err != nil {
		return false, err
	}
	n, err := s.conn.Eval(ctx, completeScript, []string{redisKey(id)}, v, owner, raw).Int()
	return n == 1, err
}

func (s *Redis) Delete(ctx context.Context, id string) error {
	return s.conn.Del(ctx, redisKey(id)).Err()
}
