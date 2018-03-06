CREATE TABLE users (
  id serial primary key not null,
  profile_id integer not null,
  access_token bytea not null,
  refresh_token bytea not null,
  preferences jsonb not null,
  internal_state jsonb not null,
  last_login timestamp with time zone,
  constraint uniq_profile_id unique(profile_id),
  constraint uniq_access_token unique(access_token),
  constraint uniq_refresh_token unique(refresh_token)
);
