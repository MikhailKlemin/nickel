CREATE TABLE statements (
    id BIGSERIAL PRIMARY KEY,
    period VARCHAR(7) NOT NULL,
    iban VARCHAR(34) NOT NULL,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (period, iban)
);
