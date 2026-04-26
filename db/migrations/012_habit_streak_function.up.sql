-- recalculate_habit_streak returns (current_streak, longest_streak) for a
-- habit, computed from its logs. Mirrors the rules formerly implemented in
-- internal/habits/service.go:
--   * No targets -> (0, h.longest_streak); caller will not invoke this anyway.
--   * recording_required=false -> carry forward the last logged value across
--     missing days before grouping into periods.
--   * Current streak walks back from today's period, skipping today if it
--     is not yet met (without breaking the streak).
--   * Longest streak is the longest run of consecutive met periods.
CREATE OR REPLACE FUNCTION recalculate_habit_streak(
    habit_id_in int,
    today_in date
) RETURNS TABLE(current_streak int, longest_streak int)
LANGUAGE plpgsql STABLE AS $$
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
    IF earliest IS NULL OR earliest > today_in THEN
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
