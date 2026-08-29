CREATE TABLE IF NOT EXISTS recurring_commitments (
  id serial PRIMARY KEY,
  task_id int NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
  label text NOT NULL CHECK (length(label) BETWEEN 1 AND 200),
  days_of_week smallint[] NOT NULL,
  start_time time NOT NULL,
  end_time time NOT NULL CHECK (end_time > start_time),
  active boolean NOT NULL DEFAULT TRUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS recurring_commitment_skips (
  commitment_id int NOT NULL REFERENCES recurring_commitments (id) ON DELETE CASCADE,
  skip_date date NOT NULL,
  PRIMARY KEY (commitment_id, skip_date)
);

ALTER TABLE plan_blocks
  ADD COLUMN commitment_id int REFERENCES recurring_commitments (id) ON DELETE SET NULL;

CREATE OR REPLACE FUNCTION recurring_commitments_touch_updated_at_fn ()
  RETURNS TRIGGER
  LANGUAGE plpgsql
  AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS recurring_commitments_touch_updated_at ON recurring_commitments;

CREATE TRIGGER recurring_commitments_touch_updated_at
  BEFORE UPDATE ON recurring_commitments
  FOR EACH ROW
  EXECUTE FUNCTION recurring_commitments_touch_updated_at_fn ();
