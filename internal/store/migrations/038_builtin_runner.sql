ALTER TABLE runner_pools ADD COLUMN builtin boolean NOT NULL DEFAULT false;
CREATE UNIQUE INDEX runner_pools_builtin ON runner_pools(org_id) WHERE builtin;

CREATE FUNCTION grant_builtin_pool() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO runner_pool_repositories(org_id,pool_id,repository_id)
    SELECT NEW.org_id,id,NEW.id FROM runner_pools WHERE org_id=NEW.org_id AND builtin
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$;
CREATE TRIGGER repository_builtin_pool AFTER INSERT ON repositories FOR EACH ROW EXECUTE FUNCTION grant_builtin_pool();
