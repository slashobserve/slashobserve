CREATE SETTINGS PROFILE IF NOT EXISTS slashobserve_ro_profile SETTINGS
    readonly = 2 CONST,
    max_execution_time = 30 MIN 1 MAX 300,
    max_result_rows = 10000 MIN 1 MAX 1000000,
    max_result_bytes = 104857600 MIN 1 MAX 1073741824,
    result_overflow_mode = 'throw' CONST;

CREATE USER IF NOT EXISTS slashobserve_ro IDENTIFIED WITH sha256_password BY '__PASSWORD__'
    SETTINGS PROFILE 'slashobserve_ro_profile';

GRANT SELECT ON otel.* TO slashobserve_ro;
