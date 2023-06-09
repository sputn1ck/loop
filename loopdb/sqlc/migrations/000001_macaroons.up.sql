CREATE TABLE IF NOT EXISTS macaroons (
    id BLOB PRIMARY KEY,
    root_key BLOB NOT NULL 
);

-- secret_key_params stores marshaled secret key parameters.
CREATE TABLE secret_key_params (
    id SERIAL PRIMARY KEY,

    -- params is a marshaled representation of the secret key parameters.
	params BYTEA
);
