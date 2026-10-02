import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { TargetForm } from './TargetForm';

afterEach(cleanup);

it('prefills only a committed exact container link and preserves a supplied name', () => {
  render(<TargetForm submitLabel="Save" onSubmit={vi.fn()} onCancel={vi.fn()} suggestions={[
    { link: 'ep/app', endpoint_name: 'host', name: 'app', image: 'app:1', state: 'running' },
    { link: 'ep/app-worker', endpoint_name: 'host', name: 'app-worker', image: 'app:1', state: 'running' },
  ]} />);
  const container = screen.getByLabelText('KyYard container');
  const name = screen.getByLabelText('Name');
  fireEvent.change(container, { target: { value: 'ep/app' } });
  expect(name).toHaveProperty('value', '');
  fireEvent.change(container, { target: { value: 'ep/app-worker' } });
  fireEvent.blur(container);
  expect(name).toHaveProperty('value', 'app-worker');
  fireEvent.change(name, { target: { value: 'My app' } });
  fireEvent.change(container, { target: { value: 'ep/app' } });
  fireEvent.blur(container);
  expect(name).toHaveProperty('value', 'My app');
  fireEvent.change(name, { target: { value: '' } });
  fireEvent.change(container, { target: { value: 'ep/app-unknown' } });
  fireEvent.blur(container);
  expect(name).toHaveProperty('value', '');
});
