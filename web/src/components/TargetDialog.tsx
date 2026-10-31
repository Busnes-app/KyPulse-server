import { useEffect, useRef } from 'react';
import { TargetForm } from './TargetForm';
import type { Suggestion, Target, TargetInput } from '../monitor';

interface TargetDialogProps {
  title: string;
  initial?: Target;
  submitLabel: string;
  suggestions?: Suggestion[];
  onSubmit: (input: TargetInput) => Promise<void>;
  onClose: () => void;
}

// TargetDialog is the add and edit form in a native modal dialog; Escape and Cancel close it.
export function TargetDialog({ title, onClose, ...form }: TargetDialogProps) {
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const d = dialog.current;
    if (d && !d.open) d.showModal();
  }, []);
  return (
    <dialog ref={dialog} className="modal-window target-dialog" aria-labelledby="target-dialog-title"
      onCancel={(event) => { event.preventDefault(); onClose(); }}>
      <h2 id="target-dialog-title">{title}</h2>
      <TargetForm {...form} onCancel={onClose} />
    </dialog>
  );
}
