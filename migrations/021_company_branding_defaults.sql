-- Ensure default company branding is present on existing stations.
UPDATE station_config
SET company_name = 'FATMA ZAHRA Services Transport',
    company_logo_url = '/assets/company-logo.png'
WHERE is_operational = true;

