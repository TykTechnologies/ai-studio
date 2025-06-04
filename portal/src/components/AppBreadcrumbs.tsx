import React from 'react';
import { Link as RouterLink, useLocation } from 'react-router-dom';
import Breadcrumbs from '@mui/material/Breadcrumbs';
import Link from '@mui/material/Link';
import Typography from '@mui/material/Typography';
import HomeIcon from '@mui/icons-material/Home'; // Example icon

const AppBreadcrumbs: React.FC = () => {
  const location = useLocation();
  const pathnames = location.pathname.split('/').filter((x) => x);

  return (
    <Breadcrumbs aria-label="breadcrumb">
      <Link
        component={RouterLink}
        underline="hover"
        sx={{ display: 'flex', alignItems: 'center' }}
        color="inherit"
        to="/"
      >
        <HomeIcon sx={{ mr: 0.5 }} fontSize="inherit" />
        Home
      </Link>
      {pathnames.map((value, index) => {
        const last = index === pathnames.length - 1;
        const to = `/${pathnames.slice(0, index + 1).join('/')}`;
        let name = value.charAt(0).toUpperCase() + value.slice(1);

        // Customize names for specific paths
        if (value === 'tools') {
          name = 'Tools Catalog';
        }
        // Add other custom names here

        return last ? (
          <Typography color="text.primary" key={to}>
            {name}
          </Typography>
        ) : (
          <Link component={RouterLink} underline="hover" color="inherit" to={to} key={to}>
            {name}
          </Link>
        );
      })}
    </Breadcrumbs>
  );
};

export default AppBreadcrumbs;
