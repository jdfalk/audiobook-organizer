// file: web/src/pages/System.tsx
// version: 1.4.0
// guid: 7c8d9e0f-1a2b-3c4d-5e6f-7a8b9c0d1e2f
// last-edited: 2026-04-30

import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Box, Typography, Paper, Tabs, Tab, Button } from '@mui/material';
import { OpenInNew as OpenInNewIcon } from '@mui/icons-material';
import { StorageTab } from '../components/system/StorageTab';
import { SystemInfoTab } from '../components/system/SystemInfoTab';
import { QuotaTab } from '../components/system/QuotaTab';
import { MaintenanceTab } from '../components/system/MaintenanceTab';

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

function TabPanel(props: TabPanelProps) {
  const { children, value, index, ...other } = props;

  return (
    <div
      role="tabpanel"
      hidden={value !== index}
      id={`system-tabpanel-${index}`}
      aria-labelledby={`system-tab-${index}`}
      {...other}
    >
      {value === index && <Box sx={{ py: 3 }}>{children}</Box>}
    </div>
  );
}

export function System() {
  const [tabValue, setTabValue] = useState(0);
  const navigate = useNavigate();

  return (
    <Box>
      <Typography variant="h4" gutterBottom>
        System
      </Typography>

      <Paper>
        <Tabs
          value={tabValue}
          onChange={(_, newValue) => setTabValue(newValue)}
          aria-label="system tabs"
        >
          <Tab label="System Info" />
          <Tab label="Storage" />
          <Tab label="Quota" />
          <Tab label="Maintenance" />
        </Tabs>

        <TabPanel value={tabValue} index={0}>
          <SystemInfoTab />
          <Box sx={{ mt: 2, px: 3, pb: 2 }}>
            <Button
              variant="outlined"
              startIcon={<OpenInNewIcon />}
              onClick={() => navigate('/activity')}
            >
              View Activity Log
            </Button>
          </Box>
        </TabPanel>

        <TabPanel value={tabValue} index={1}>
          <StorageTab />
        </TabPanel>

        <TabPanel value={tabValue} index={2}>
          <QuotaTab />
        </TabPanel>

        <TabPanel value={tabValue} index={3}>
          <MaintenanceTab />
        </TabPanel>
      </Paper>
    </Box>
  );
}
