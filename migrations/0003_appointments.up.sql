-- DentaCore: appointments.
-- The use case checks the calendar (working day, opening hours, slot grid); the database owns
-- the one rule a check-then-insert cannot enforce under concurrency: no double booking.

-- btree_gist lets a GiST exclusion constraint compare uuid with =. It is a trusted extension,
-- so the database owner can install it without superuser. Pinned to public so the operator
-- classes resolve no matter which schema a session migrates into.
CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public;

CREATE TABLE appointments (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    -- RESTRICT: an appointment is part of the medical history; deleting a user must not erase it.
    patient_id    uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    doctor_id     uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_by    uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    starts_at     timestamptz NOT NULL,
    ends_at       timestamptz NOT NULL,
    status        text        NOT NULL DEFAULT 'scheduled',
    reason        text        NOT NULL DEFAULT '',
    cancel_reason text        NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT appointments_ends_after_start CHECK (ends_at > starts_at),
    CONSTRAINT appointments_length_bounded CHECK (ends_at - starts_at <= interval '8 hours'),
    CONSTRAINT appointments_status_known CHECK (
        status IN ('scheduled', 'confirmed', 'completed', 'cancelled', 'no_show')
    ),
    CONSTRAINT appointments_distinct_parties CHECK (patient_id <> doctor_id),
    CONSTRAINT appointments_reason_length CHECK (char_length(reason) <= 500),
    CONSTRAINT appointments_cancel_reason_length CHECK (char_length(cancel_reason) <= 500),

    -- Half-open ranges: a visit ending at 10:00 does not collide with one starting at 10:00.
    -- Only live bookings hold a slot, so a cancelled visit frees it for someone else.
    CONSTRAINT appointments_doctor_no_overlap EXCLUDE USING gist (
        doctor_id WITH =,
        tstzrange(starts_at, ends_at, '[)') WITH &&
    ) WHERE (status IN ('scheduled', 'confirmed')),
    CONSTRAINT appointments_patient_no_overlap EXCLUDE USING gist (
        patient_id WITH =,
        tstzrange(starts_at, ends_at, '[)') WITH &&
    ) WHERE (status IN ('scheduled', 'confirmed'))
);

-- A doctor's day view and the busy-interval lookup behind free slots.
CREATE INDEX appointments_doctor_starts_at_idx ON appointments (doctor_id, starts_at);
-- A patient's own history.
CREATE INDEX appointments_patient_starts_at_idx ON appointments (patient_id, starts_at);
-- The admin's clinic-wide list, paged in start order.
CREATE INDEX appointments_starts_at_id_idx ON appointments (starts_at, id);
