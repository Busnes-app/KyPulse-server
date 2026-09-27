import React from 'react';

export interface PageUser { role: string }

export const Status: React.FC<{ user: PageUser; onChanged: () => void }> = () => (
  <div className="dr-page"><h1>Status</h1></div>
);
