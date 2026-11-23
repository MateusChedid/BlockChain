import axios from 'axios';

const http = axios.create({ baseURL: '/api' });

// Organization that signs the transactions (sent as the X-Org header).
let currentOrg = 'utility';
export const setSigningOrg = (org) => { currentOrg = org; };
http.interceptors.request.use((config) => {
  config.headers['X-Org'] = currentOrg;
  return config;
});

// Extracts the chaincode error message returned by the API.
export const errorMessage = (err) =>
  err?.response?.data?.error || err?.message || 'Unknown error';

export const api = {
  network: () => http.get('/network'),

  governance: {
    listMembers: () => http.get('/governance/members'),
    onboardMember: (data) => http.post('/governance/members', data),
    getTariffs: () => http.get('/governance/tariffs'),
    updateTariffs: (data) => http.put('/governance/tariffs', data),
    getSandbox: () => http.get('/governance/sandbox'),
    setSandbox: (data) => http.put('/governance/sandbox', data),
    getPriceBand: () => http.get('/governance/price-band'),
  },

  // Demonstrated scope
  meters: {
    list: () => http.get('/meters'),
    register: (data) => http.post('/meters', data),
    submitReading: (meterID, data) => http.post(`/meters/${meterID}/readings`, data),
    deactivate: (meterID) => http.post(`/meters/${meterID}/deactivate`),
    simulator: (meterID) => http.get(`/simulator/${meterID}`),
  },
  metering: {
    readings: (meterID = '') => http.get('/readings', { params: { meterID } }),
    states: () => http.get('/meter-states'),
    reconcile: (data) => http.post('/reconciliations', data),
    reconciliations: () => http.get('/reconciliations'),
    audit: (cycle, substation = '') => http.get('/audit', { params: { cycle, substation } }),
    simulateBill: (data) => http.post('/billing/simulate', data),
    statements: () => http.get('/billing/statements'),
    accounts: () => http.get('/accounts'),
    account: (owner) => http.get(`/accounts/${owner}`),
    history: (owner) => http.get(`/accounts/${owner}/history`),
  },

  // Future scope (refused unless the regulatory sandbox is on)
  sandbox: {
    listOrders: (substation = '', status = '') => http.get('/sandbox/orders', { params: { substation, status } }),
    submitAsk: (data) => http.post('/sandbox/asks', data),
    submitBid: (data) => http.post('/sandbox/bids', data),
    cancel: (orderID) => http.post(`/sandbox/orders/${orderID}/cancel`),
    listTrades: () => http.get('/sandbox/trades'),
  },

  users: {
    list: () => http.get('/users'),
    create: (data) => http.post('/users', data),
  },
};
