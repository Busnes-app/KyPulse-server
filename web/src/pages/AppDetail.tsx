import React from 'react';
import type { PageUser } from './Status';

export const AppDetail: React.FC<{ id: string; user: PageUser; onChanged: () => void }> = ({ id }) => (
  <div className="dr-page"><h1>{id}</h1></div>
);
