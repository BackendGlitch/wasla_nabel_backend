-- Add company info to station_config for the /init bootstrap endpoint.
ALTER TABLE station_config ADD COLUMN IF NOT EXISTS company_name TEXT NOT NULL DEFAULT '';
ALTER TABLE station_config ADD COLUMN IF NOT EXISTS company_logo_url TEXT NOT NULL DEFAULT '';

UPDATE station_config
SET company_name = 'STE Dhraiff Services Transport',
    company_logo_url = '/assets/company-logo.png'
WHERE company_name = '';
