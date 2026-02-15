// file: web/src/pages/System.tsx
// version: 1.2.0
// guid: 7c8d9e0f-1a2b-3c4d-5e6f-7a8b9c0d1e2f

import { useState } from 'react';
import { Box, Typography, Paper, Tabs, Tab } from '@mui/material';
import { LogsTab } from '../components/system/LogsTab';
import { StorageTab } from '../components/system/StorageTab';
import { SystemInfoTab } from '../components/system/SystemInfoTab';
import { QuotaTab } from '../components/system/QuotaTab';

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
          <Tab label="Logs" />
        </Tabs>

        <TabPanel value={tabValue} index={0}>
          <SystemInfoTab />
        </TabPanel>

        <TabPanel value={tabValue} index={1}>
          <StorageTab />
        </TabPanel>

        <TabPanel value={tabValue} index={2}>
          <QuotaTab />
        </TabPanel>

        <TabPanel value={tabValue} index={3}>
          <LogsTab />
        </TabPanel>
      </Paper>
    </Box>
  );
}
