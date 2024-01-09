import React from 'react';
import { render, fireEvent, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import App from '../src/App';

test('renders App component with routes', () => {
  render(<App />);

  const jobsDashboardElement = screen.getAllByText(/Jobs Dashboard/i);
  expect(jobsDashboardElement.length).toBeGreaterThan(0);

  const nodesDashboardElement = screen.getAllByText(/Nodes Dashboard/i);
  expect(nodesDashboardElement.length).toBeGreaterThan(0);

  const settingsElement = screen.getAllByText(/Settings/i);
  expect(settingsElement.length).toBeGreaterThan(0);
});
