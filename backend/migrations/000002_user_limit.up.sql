CREATE OR REPLACE FUNCTION enforce_homeflow_user_limit()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF (SELECT count(*) FROM users) >= 2 THEN
        RAISE EXCEPTION 'HomeFlow supports exactly two user accounts';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER users_maximum_two
BEFORE INSERT ON users
FOR EACH ROW EXECUTE FUNCTION enforce_homeflow_user_limit();

