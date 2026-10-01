package browserstate

import "context"

func (s *Memory) ReleaseClaim(_ context.Context, id string, version int64, owner string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, found := s.records[id]
	if !found || owner == "" || record.Version != version || record.Owner != owner {
		return false, nil
	}
	record.Owner = ""
	s.records[id] = record
	return true, nil
}

const releaseClaimScript = `
local raw=redis.call('GET',KEYS[1])
if not raw then return 0 end
local r=cjson.decode(raw)
if ARGV[2]=='' or r.version~=tonumber(ARGV[1]) or r.owner~=ARGV[2] then return 0 end
r.owner=''
redis.call('SET',KEYS[1],cjson.encode(r),'KEEPTTL')
return 1`

func (s *Redis) ReleaseClaim(ctx context.Context, id string, version int64, owner string) (bool, error) {
	n, err := s.conn.Eval(ctx, releaseClaimScript, []string{redisKey(id)}, version, owner).Int()
	return n == 1, err
}

func (s *Postgres) ReleaseClaim(ctx context.Context, id string, version int64, owner string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE goadmin_browser_auth_state
 SET owner='', record=jsonb_set(record,'{owner}',to_jsonb(''::text))
 WHERE id=$1 AND version=$2 AND owner=$3 AND owner<>'' AND expires_at>now()`, id, version, owner)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
