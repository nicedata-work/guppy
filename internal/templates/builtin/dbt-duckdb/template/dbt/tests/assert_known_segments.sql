-- Singular test: fails if dim_customer carries an unknown segment.
select distinct segment
from {{ ref('dim_customer') }}
where segment not in ('enterprise', 'smb')
