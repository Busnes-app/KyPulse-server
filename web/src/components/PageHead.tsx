import React, { createContext, useContext } from 'react';

/** The app-wide status banner; every page head renders it right below itself. */
export const PageBanner = createContext<React.ReactNode>(null);

/** The suite page head (ky-ui shell): the page's one h1, a one-line lede and its primary action. */
export function PageHead({ title, lede, action }: { title: React.ReactNode; lede?: React.ReactNode; action?: React.ReactNode }) {
  return (
    <>
    <header className="ky-page-head">
      <div>
        <h1 className="ky-page-title">{title}</h1>
        {lede && <p className="ky-page-lede">{lede}</p>}
      </div>
      {action}
    </header>
    {useContext(PageBanner)}
    </>
  );
}
