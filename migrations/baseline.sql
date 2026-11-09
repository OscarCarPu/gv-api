-- gv-api schema baseline: the Go release (d32ab42) at golang-migrate version 036.
-- pg_dump --schema-only --no-owner --no-privileges --exclude-table=schema_migrations of an
-- empty database migrated by that release, with the session SETs removed. Run by revision
-- 0001_baseline, which also inserts the row migration 036 seeded. Never edit: add revisions.

SET LOCAL check_function_bodies = false;

CREATE TYPE public.outage_kind AS ENUM (
    'start',
    'up',
    'down',
    'failover'
);

CREATE TYPE public.transaction_type AS ENUM (
    'income',
    'expense',
    'transfer'
);

CREATE FUNCTION public.calendar_touch_updated_at_fn() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

CREATE FUNCTION public.plan_blocks_touch_updated_at_fn() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

CREATE FUNCTION public.recalculate_habit_streak(habit_id_in integer, today_in date) RETURNS TABLE(current_streak integer, longest_streak integer)
    LANGUAGE plpgsql STABLE
    AS $$
DECLARE
    h habits%ROWTYPE;
    earliest date;
    trunc_unit text;
BEGIN
    SELECT * INTO h FROM habits WHERE id = habit_id_in;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'habit % not found', habit_id_in;
    END IF;

    -- habits.frequency stores 'daily'/'weekly'/'monthly'; date_trunc/interval need 'day'/'week'/'month'.
    trunc_unit := CASE h.frequency
        WHEN 'daily' THEN 'day'
        WHEN 'weekly' THEN 'week'
        WHEN 'monthly' THEN 'month'
    END;

    IF h.target_min IS NULL AND h.target_max IS NULL THEN
        RETURN QUERY SELECT 0, h.longest_streak;
        RETURN;
    END IF;

    SELECT MIN(log_date) INTO earliest FROM habit_logs WHERE habit_id = habit_id_in;
    IF earliest IS NULL THEN
        earliest := h.created_at::date;
    END IF;
    IF earliest > today_in THEN
        RETURN QUERY SELECT 0, 0;
        RETURN;
    END IF;

    RETURN QUERY
    WITH days AS (
        SELECT generate_series(earliest, today_in, interval '1 day')::date AS day
    ),
    day_values AS (
        SELECT d.day,
            CASE WHEN h.recording_required THEN
                COALESCE((SELECT hl.value FROM habit_logs hl
                          WHERE hl.habit_id = habit_id_in AND hl.log_date = d.day), 0)
            ELSE
                COALESCE((SELECT hl.value FROM habit_logs hl
                          WHERE hl.habit_id = habit_id_in AND hl.log_date <= d.day
                          ORDER BY hl.log_date DESC LIMIT 1), 0)
            END AS value
        FROM days d
    ),
    period_sums AS (
        SELECT date_trunc(trunc_unit, dv.day::timestamp)::date AS period_start,
               SUM(dv.value)::real AS sum
        FROM day_values dv
        GROUP BY 1
    ),
    today_period AS (
        SELECT date_trunc(trunc_unit, today_in::timestamp)::date AS p
    ),
    all_periods AS (
        SELECT generate_series(
            (SELECT MIN(period_start) FROM period_sums),
            (SELECT p FROM today_period),
            ('1 ' || trunc_unit)::interval
        )::date AS period_start
    ),
    flagged AS (
        SELECT ap.period_start,
            CASE WHEN
                (h.target_min IS NULL OR COALESCE(ps.sum, 0) >= h.target_min)
                AND (h.target_max IS NULL OR COALESCE(ps.sum, 0) <= h.target_max)
            THEN 1 ELSE 0 END AS met
        FROM all_periods ap LEFT JOIN period_sums ps ON ps.period_start = ap.period_start
    ),
    grouped AS (
        SELECT period_start, met,
            SUM(CASE WHEN met = 1 THEN 0 ELSE 1 END) OVER (ORDER BY period_start) AS grp
        FROM flagged
    ),
    runs AS (
        SELECT grp, COUNT(*)::int AS len, MAX(period_start) AS run_end
        FROM grouped WHERE met = 1
        GROUP BY grp
    ),
    current_met AS (
        SELECT met FROM flagged WHERE period_start = (SELECT p FROM today_period)
    )
    SELECT
        CASE
            WHEN COALESCE((SELECT met FROM current_met), 0) = 1
                THEN COALESCE((SELECT len FROM runs WHERE run_end = (SELECT p FROM today_period)), 0)
            ELSE COALESCE((SELECT len FROM runs
                           WHERE run_end = ((SELECT p FROM today_period) - ('1 ' || trunc_unit)::interval)::date), 0)
        END AS current_streak,
        COALESCE((SELECT MAX(len) FROM runs), 0) AS longest_streak;
END;
$$;

CREATE FUNCTION public.recurring_commitments_touch_updated_at_fn() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;

CREATE FUNCTION public.task_dependency_would_cycle(task_id_in integer, new_deps integer[]) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
    WITH RECURSIVE reach(node) AS (
        SELECT * FROM unnest(new_deps)
        UNION
        SELECT td.depends_on
        FROM task_dependencies td, reach r
        WHERE td.task_id = r.node AND td.task_id != task_id_in
    )
    SELECT EXISTS(SELECT 1 FROM reach r WHERE r.node = task_id_in);
$$;

CREATE FUNCTION public.transactions_apply_total_fn() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF TG_OP IN ('UPDATE','DELETE') THEN
        IF OLD.type = 'income' THEN
            UPDATE accounts SET total = total - OLD.amount WHERE id = OLD.account_id;
        ELSIF OLD.type = 'expense' THEN
            UPDATE accounts SET total = total + OLD.amount WHERE id = OLD.account_id;
        ELSIF OLD.type = 'transfer' THEN
            UPDATE accounts SET total = total + OLD.amount WHERE id = OLD.account_id;
            UPDATE accounts SET total = total - OLD.amount WHERE id = OLD.to_account_id;
        END IF;
    END IF;

    IF TG_OP IN ('INSERT','UPDATE') THEN
        IF NEW.type = 'income' THEN
            UPDATE accounts SET total = total + NEW.amount WHERE id = NEW.account_id;
        ELSIF NEW.type = 'expense' THEN
            UPDATE accounts SET total = total - NEW.amount WHERE id = NEW.account_id;
        ELSIF NEW.type = 'transfer' THEN
            UPDATE accounts SET total = total - NEW.amount WHERE id = NEW.account_id;
            UPDATE accounts SET total = total + NEW.amount WHERE id = NEW.to_account_id;
        END IF;
    END IF;

    RETURN NULL;
END;
$$;

CREATE TABLE public.accounts (
    id integer NOT NULL,
    name text NOT NULL,
    total numeric(15,2) DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT accounts_name_check CHECK (((length(name) >= 1) AND (length(name) <= 40)))
);

CREATE SEQUENCE public.accounts_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.accounts_id_seq OWNED BY public.accounts.id;

CREATE TABLE public.budgets (
    id integer NOT NULL,
    category_id integer NOT NULL,
    month date NOT NULL,
    amount numeric(15,2),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    period text DEFAULT 'monthly'::text NOT NULL,
    CONSTRAINT budgets_amount_check CHECK (((amount IS NULL) OR (amount >= (0)::numeric))),
    CONSTRAINT budgets_month_check CHECK ((EXTRACT(day FROM month) = (1)::numeric)),
    CONSTRAINT budgets_period_check CHECK ((period = ANY (ARRAY['monthly'::text, 'yearly'::text]))),
    CONSTRAINT budgets_yearly_january_check CHECK (((period = 'monthly'::text) OR (EXTRACT(month FROM month) = (1)::numeric)))
);

CREATE SEQUENCE public.budgets_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.budgets_id_seq OWNED BY public.budgets.id;

CREATE TABLE public.calendar_events (
    id integer NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    start_time timestamp with time zone NOT NULL,
    end_time timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE SEQUENCE public.calendar_events_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.calendar_events_id_seq OWNED BY public.calendar_events.id;

CREATE TABLE public.categories (
    id integer NOT NULL,
    name text NOT NULL,
    parent_id integer,
    type public.transaction_type NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT categories_check CHECK (((parent_id IS NULL) OR (parent_id <> id))),
    CONSTRAINT categories_name_check CHECK (((length(name) >= 1) AND (length(name) <= 40)))
);

CREATE SEQUENCE public.categories_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.categories_id_seq OWNED BY public.categories.id;

CREATE TABLE public.concello_marks (
    id integer NOT NULL,
    name text NOT NULL,
    visited_on date NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE public.concello_marks ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.concello_marks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

CREATE TABLE public.habit_logs (
    habit_id integer NOT NULL,
    log_date date DEFAULT CURRENT_DATE NOT NULL,
    value real NOT NULL
);

CREATE TABLE public.habits (
    id integer NOT NULL,
    name character varying(40) NOT NULL,
    description text,
    frequency text DEFAULT 'daily'::text NOT NULL,
    current_streak integer DEFAULT 0 NOT NULL,
    longest_streak integer DEFAULT 0 NOT NULL,
    target_min real,
    target_max real,
    recording_required boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT habits_frequency_check CHECK ((frequency = ANY (ARRAY['daily'::text, 'weekly'::text, 'monthly'::text])))
);

ALTER TABLE public.habits ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.habits_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

CREATE TABLE public.lights (
    id text NOT NULL,
    name text NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    address text NOT NULL,
    protocol text NOT NULL,
    supports_color boolean DEFAULT true NOT NULL,
    supports_color_temp boolean DEFAULT true NOT NULL,
    min_color_temp double precision DEFAULT 2200 NOT NULL,
    max_color_temp double precision DEFAULT 6500 NOT NULL,
    options jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.outages (
    id bigint NOT NULL,
    ends_at timestamp with time zone NOT NULL,
    kind public.outage_kind NOT NULL
);

ALTER TABLE public.outages ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.outages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

CREATE TABLE public.plan_blocks (
    id integer NOT NULL,
    plan_date date NOT NULL,
    started_at timestamp with time zone NOT NULL,
    ended_at timestamp with time zone NOT NULL,
    task_id integer,
    label text NOT NULL,
    note text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    event_ref text,
    commitment_id integer,
    CONSTRAINT plan_blocks_check CHECK ((ended_at > started_at)),
    CONSTRAINT plan_blocks_label_check CHECK (((length(label) >= 1) AND (length(label) <= 200)))
);

CREATE SEQUENCE public.plan_blocks_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.plan_blocks_id_seq OWNED BY public.plan_blocks.id;

CREATE TABLE public.projects (
    id integer NOT NULL,
    parent_id integer,
    name character varying(40) NOT NULL,
    description text,
    due_at date,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    priority integer DEFAULT 3 NOT NULL,
    CONSTRAINT chk_project_priority_range CHECK (((priority >= 1) AND (priority <= 5)))
);

CREATE SEQUENCE public.projects_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.projects_id_seq OWNED BY public.projects.id;

CREATE TABLE public.recurring_commitment_skips (
    commitment_id integer NOT NULL,
    skip_date date NOT NULL
);

CREATE TABLE public.recurring_commitments (
    id integer NOT NULL,
    task_id integer NOT NULL,
    label text NOT NULL,
    days_of_week smallint[] NOT NULL,
    start_time time without time zone NOT NULL,
    end_time time without time zone NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT recurring_commitments_check CHECK ((end_time > start_time)),
    CONSTRAINT recurring_commitments_label_check CHECK (((length(label) >= 1) AND (length(label) <= 200)))
);

CREATE SEQUENCE public.recurring_commitments_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.recurring_commitments_id_seq OWNED BY public.recurring_commitments.id;

CREATE TABLE public.task_dependencies (
    task_id integer NOT NULL,
    depends_on integer NOT NULL,
    CONSTRAINT task_dependencies_check CHECK ((task_id <> depends_on))
);

CREATE TABLE public.tasks (
    id integer NOT NULL,
    project_id integer,
    name character varying(40) NOT NULL,
    description text,
    due_at date,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    task_type text DEFAULT 'standard'::text NOT NULL,
    recurrence integer,
    priority integer DEFAULT 3 NOT NULL,
    estimate_hours numeric(5,2),
    CONSTRAINT chk_priority_range CHECK (((priority >= 1) AND (priority <= 5))),
    CONSTRAINT chk_recurrence_positive CHECK (((recurrence IS NULL) OR (recurrence > 0))),
    CONSTRAINT chk_recurrence_required CHECK ((((task_type = 'recurring'::text) AND (recurrence IS NOT NULL)) OR ((task_type <> 'recurring'::text) AND (recurrence IS NULL)))),
    CONSTRAINT chk_task_type_values CHECK ((task_type = ANY (ARRAY['standard'::text, 'continuous'::text, 'recurring'::text]))),
    CONSTRAINT tasks_estimate_hours_check CHECK (((estimate_hours IS NULL) OR (estimate_hours > (0)::numeric)))
);

CREATE VIEW public.task_effective_due AS
 WITH RECURSIVE blocks_closure(root_id, descendant_due, descendant_id) AS (
         SELECT t.id,
            t.due_at,
            t.id
           FROM public.tasks t
          WHERE (t.finished_at IS NULL)
        UNION
         SELECT bc.root_id,
            t2.due_at,
            td.task_id
           FROM ((blocks_closure bc
             JOIN public.task_dependencies td ON ((td.depends_on = bc.descendant_id)))
             JOIN public.tasks t2 ON (((t2.id = td.task_id) AND (t2.finished_at IS NULL))))
        )
 SELECT blocks_closure.root_id AS task_id,
    min(blocks_closure.descendant_due) AS effective_due_at
   FROM blocks_closure
  GROUP BY blocks_closure.root_id;

CREATE SEQUENCE public.tasks_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.tasks_id_seq OWNED BY public.tasks.id;

CREATE TABLE public.time_entries (
    id integer NOT NULL,
    task_id integer NOT NULL,
    started_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    finished_at timestamp with time zone,
    comment text
);

CREATE SEQUENCE public.time_entries_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.time_entries_id_seq OWNED BY public.time_entries.id;

CREATE TABLE public.todos (
    id integer NOT NULL,
    task_id integer NOT NULL,
    name character varying(40) NOT NULL,
    is_done boolean DEFAULT false NOT NULL
);

CREATE SEQUENCE public.todos_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.todos_id_seq OWNED BY public.todos.id;

CREATE TABLE public.transactions (
    id integer NOT NULL,
    type public.transaction_type NOT NULL,
    amount numeric(15,2) NOT NULL,
    account_id integer NOT NULL,
    to_account_id integer,
    description text,
    occurred_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    category_id integer,
    CONSTRAINT transactions_amount_check CHECK ((amount > (0)::numeric)),
    CONSTRAINT transactions_type_layout_check CHECK ((((type = 'transfer'::public.transaction_type) AND (to_account_id IS NOT NULL) AND (to_account_id <> account_id)) OR ((type = ANY (ARRAY['income'::public.transaction_type, 'expense'::public.transaction_type])) AND (to_account_id IS NULL))))
);

CREATE SEQUENCE public.transactions_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.transactions_id_seq OWNED BY public.transactions.id;

ALTER TABLE ONLY public.accounts ALTER COLUMN id SET DEFAULT nextval('public.accounts_id_seq'::regclass);

ALTER TABLE ONLY public.budgets ALTER COLUMN id SET DEFAULT nextval('public.budgets_id_seq'::regclass);

ALTER TABLE ONLY public.calendar_events ALTER COLUMN id SET DEFAULT nextval('public.calendar_events_id_seq'::regclass);

ALTER TABLE ONLY public.categories ALTER COLUMN id SET DEFAULT nextval('public.categories_id_seq'::regclass);

ALTER TABLE ONLY public.plan_blocks ALTER COLUMN id SET DEFAULT nextval('public.plan_blocks_id_seq'::regclass);

ALTER TABLE ONLY public.projects ALTER COLUMN id SET DEFAULT nextval('public.projects_id_seq'::regclass);

ALTER TABLE ONLY public.recurring_commitments ALTER COLUMN id SET DEFAULT nextval('public.recurring_commitments_id_seq'::regclass);

ALTER TABLE ONLY public.tasks ALTER COLUMN id SET DEFAULT nextval('public.tasks_id_seq'::regclass);

ALTER TABLE ONLY public.time_entries ALTER COLUMN id SET DEFAULT nextval('public.time_entries_id_seq'::regclass);

ALTER TABLE ONLY public.todos ALTER COLUMN id SET DEFAULT nextval('public.todos_id_seq'::regclass);

ALTER TABLE ONLY public.transactions ALTER COLUMN id SET DEFAULT nextval('public.transactions_id_seq'::regclass);

ALTER TABLE ONLY public.accounts
    ADD CONSTRAINT accounts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.budgets
    ADD CONSTRAINT budgets_category_id_period_month_key UNIQUE (category_id, period, month);

ALTER TABLE ONLY public.budgets
    ADD CONSTRAINT budgets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.calendar_events
    ADD CONSTRAINT calendar_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.categories
    ADD CONSTRAINT categories_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.concello_marks
    ADD CONSTRAINT concello_marks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.habit_logs
    ADD CONSTRAINT habit_logs_habit_id_log_date_key UNIQUE (habit_id, log_date);

ALTER TABLE ONLY public.habits
    ADD CONSTRAINT habits_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.lights
    ADD CONSTRAINT lights_address_key UNIQUE (address);

ALTER TABLE ONLY public.lights
    ADD CONSTRAINT lights_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.outages
    ADD CONSTRAINT outages_ends_at_key UNIQUE (ends_at);

ALTER TABLE ONLY public.outages
    ADD CONSTRAINT outages_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.plan_blocks
    ADD CONSTRAINT plan_blocks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.recurring_commitment_skips
    ADD CONSTRAINT recurring_commitment_skips_pkey PRIMARY KEY (commitment_id, skip_date);

ALTER TABLE ONLY public.recurring_commitments
    ADD CONSTRAINT recurring_commitments_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.task_dependencies
    ADD CONSTRAINT task_dependencies_pkey PRIMARY KEY (task_id, depends_on);

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.time_entries
    ADD CONSTRAINT time_entries_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.todos
    ADD CONSTRAINT todos_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.transactions
    ADD CONSTRAINT transactions_pkey PRIMARY KEY (id);

CREATE INDEX idx_calendar_events_time ON public.calendar_events USING btree (start_time, end_time);

CREATE INDEX idx_categories_parent ON public.categories USING btree (parent_id);

CREATE INDEX idx_categories_type ON public.categories USING btree (type);

CREATE INDEX idx_plan_blocks_date ON public.plan_blocks USING btree (plan_date, started_at);

CREATE UNIQUE INDEX idx_plan_blocks_event_ref ON public.plan_blocks USING btree (event_ref) WHERE (event_ref IS NOT NULL);

CREATE INDEX idx_plan_blocks_task ON public.plan_blocks USING btree (task_id) WHERE (task_id IS NOT NULL);

CREATE INDEX idx_projects_parent_id ON public.projects USING btree (parent_id);

CREATE INDEX idx_task_dependencies_depends_on ON public.task_dependencies USING btree (depends_on);

CREATE INDEX idx_tasks_due_at ON public.tasks USING btree (due_at) WHERE (due_at IS NOT NULL);

CREATE INDEX idx_tasks_project_id ON public.tasks USING btree (project_id);

CREATE INDEX idx_tasks_unfinished ON public.tasks USING btree (finished_at) WHERE (finished_at IS NULL);

CREATE UNIQUE INDEX idx_time_entries_one_active ON public.time_entries USING btree ((true)) WHERE (finished_at IS NULL);

CREATE INDEX idx_time_entries_started_at ON public.time_entries USING btree (started_at);

CREATE INDEX idx_time_entries_task_id ON public.time_entries USING btree (task_id);

CREATE INDEX idx_todos_task_id ON public.todos USING btree (task_id);

CREATE INDEX idx_transactions_account ON public.transactions USING btree (account_id, occurred_at DESC);

CREATE INDEX idx_transactions_category ON public.transactions USING btree (category_id, occurred_at DESC);

CREATE INDEX idx_transactions_to_account ON public.transactions USING btree (to_account_id, occurred_at DESC) WHERE (to_account_id IS NOT NULL);

CREATE UNIQUE INDEX plan_blocks_commitment_date_uidx ON public.plan_blocks USING btree (commitment_id, plan_date) WHERE (commitment_id IS NOT NULL);

CREATE TRIGGER calendar_events_touch_updated_at BEFORE UPDATE ON public.calendar_events FOR EACH ROW EXECUTE FUNCTION public.calendar_touch_updated_at_fn();

CREATE TRIGGER plan_blocks_touch_updated_at BEFORE UPDATE ON public.plan_blocks FOR EACH ROW EXECUTE FUNCTION public.plan_blocks_touch_updated_at_fn();

CREATE TRIGGER recurring_commitments_touch_updated_at BEFORE UPDATE ON public.recurring_commitments FOR EACH ROW EXECUTE FUNCTION public.recurring_commitments_touch_updated_at_fn();

CREATE TRIGGER transactions_apply_total AFTER INSERT OR DELETE OR UPDATE ON public.transactions FOR EACH ROW EXECUTE FUNCTION public.transactions_apply_total_fn();

ALTER TABLE ONLY public.budgets
    ADD CONSTRAINT budgets_category_id_fkey FOREIGN KEY (category_id) REFERENCES public.categories(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.categories
    ADD CONSTRAINT categories_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.categories(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.habit_logs
    ADD CONSTRAINT fk_habit FOREIGN KEY (habit_id) REFERENCES public.habits(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.plan_blocks
    ADD CONSTRAINT plan_blocks_commitment_id_fkey FOREIGN KEY (commitment_id) REFERENCES public.recurring_commitments(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.plan_blocks
    ADD CONSTRAINT plan_blocks_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.projects(id);

ALTER TABLE ONLY public.recurring_commitment_skips
    ADD CONSTRAINT recurring_commitment_skips_commitment_id_fkey FOREIGN KEY (commitment_id) REFERENCES public.recurring_commitments(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.recurring_commitments
    ADD CONSTRAINT recurring_commitments_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.task_dependencies
    ADD CONSTRAINT task_dependencies_depends_on_fkey FOREIGN KEY (depends_on) REFERENCES public.tasks(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.task_dependencies
    ADD CONSTRAINT task_dependencies_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id);

ALTER TABLE ONLY public.time_entries
    ADD CONSTRAINT time_entries_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.todos
    ADD CONSTRAINT todos_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.transactions
    ADD CONSTRAINT transactions_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.transactions
    ADD CONSTRAINT transactions_category_id_fkey FOREIGN KEY (category_id) REFERENCES public.categories(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.transactions
    ADD CONSTRAINT transactions_to_account_id_fkey FOREIGN KEY (to_account_id) REFERENCES public.accounts(id) ON DELETE RESTRICT;
