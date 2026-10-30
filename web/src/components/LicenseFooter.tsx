import React, { useEffect, useRef, useState } from 'react';
import mitLicenseText from '../mit.txt?raw';

interface LicenseFooterProps {
  appName: string;
  // version comes from the authenticated /api/status; empty hides the badge.
  version?: string;
}

// The suite's copyright line and About dialog, matching KyPost.
export const LicenseFooter: React.FC<LicenseFooterProps> = ({ appName, version }) => {
  const [open, setOpen] = useState(false);
  const dialogRef = useRef<HTMLDialogElement>(null);
  const year = new Date().getFullYear();

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    if (open && !dialog.open) dialog.showModal();
    else if (!open && dialog.open) dialog.close();
  }, [open]);

  return (
    <>
      <div className="sidebar-footer">
        <p>
          <button type="button" className="license-link" onClick={() => setOpen(true)}>
            &copy; {year} &middot; MIT License
          </button>
        </p>
      </div>
      <dialog
        ref={dialogRef}
        className="compose-backdrop"
        aria-label={`About ${appName}`}
        onCancel={() => setOpen(false)}
        onClick={(event) => {
          if (event.target === dialogRef.current) setOpen(false);
        }}
      >
        <div className="license-window">
          <div className="license-window-header">
            <div className="license-window-title">
              <div className="license-title-main">
                <span className="license-app-name">{appName}</span>
                {version ? <span className="license-version-badge">v{version}</span> : null}
              </div>
              <p className="license-title-sub">Developed by Busnes.app</p>
              <p className="license-title-sub">&copy; {year} &middot; Licensed under the MIT License</p>
            </div>
            <button type="button" className="btn-secondary" onClick={() => setOpen(false)}>
              Close
            </button>
          </div>
          <textarea className="license-text" readOnly value={mitLicenseText} aria-label="MIT License" />
        </div>
      </dialog>
    </>
  );
};
