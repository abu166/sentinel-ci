import { useState } from "react";
import {
  Check,
  GitBranch,
  ShieldCheck,
  KeyRound,
  RefreshCw,
  Eye,
  EyeOff,
  Copy,
} from "lucide-react";
import { Repository } from "../model";
export default function RepositoryEditor({
  repo,
  disabled,
  onSave,
}: {
  repo: Repository;
  disabled: boolean;
  onSave: (values: Record<string, unknown>) => Promise<void>;
}) {
  const [branch, setBranch] = useState(repo.default_branch),
    [secret, setSecret] = useState(""),
    [auto, setAuto] = useState(repo.auto_commit ?? false),
    [pr, setPr] = useState(repo.open_pr ?? false),
    [manual, setManual] = useState(repo.manual_approval ?? true),
    [dirty, setDirty] = useState(false),
    [reveal, setReveal] = useState(false),
    [saveError, setSaveError] = useState("");
  const changed = () => setDirty(true);
  return (
    <form
      className="repository"
      onSubmit={(e) => {
        e.preventDefault();
        setSaveError("");
        void onSave({
          default_branch: branch,
          ...(secret ? { webhook_secret: secret } : {}),
          auto_commit: auto,
          open_pr: pr,
          manual_approval: manual,
        })
          .then(() => {
            setSecret("");
            setDirty(false);
          })
          .catch(() =>
            setSaveError(
              "Settings were not saved. Review the gateway error and retry.",
            ),
          );
      }}
    >
      <div className="repository-heading">
        <GitBranch size={17} />
        <div>
          <h2>{repo.name}</h2>
          <a href={repo.repo_url} target="_blank" rel="noreferrer">
            {repo.repo_url.replace("https://", "")}
          </a>
        </div>
        <div className="reliability">
          <strong>
            {repo.reliability_score === undefined &&
            repo.success_rate === undefined
              ? "—"
              : `${repo.reliability_score ?? repo.success_rate}%`}
          </strong>
          <small>reliability</small>
        </div>
      </div>
      <div className="repo-fields">
        <label>
          Default branch
          <input
            value={branch}
            disabled={disabled}
            onChange={(e) => {
              setBranch(e.target.value);
              changed();
            }}
            required
          />
        </label>
        <label className="webhook-field">
          Webhook secret
          <span className="secret-state">
            {repo.webhook_configured || repo.webhook_secret_configured
              ? "configured · redacted"
              : "not configured"}
          </span>
          <input
            type={reveal ? "text" : "password"}
            aria-label={`Webhook secret for ${repo.name}`}
            value={secret}
            disabled={disabled}
            onChange={(e) => {
              setSecret(e.target.value);
              changed();
            }}
            placeholder="Set or rotate secret"
            autoComplete="new-password"
          />
          <span className="secret-tools">
            <button
              type="button"
              disabled={disabled}
              aria-label={`Generate webhook secret for ${repo.name}`}
              onClick={() => {
                const bytes = crypto.getRandomValues(new Uint8Array(32));
                setSecret(
                  Array.from(bytes, (b) =>
                    b.toString(16).padStart(2, "0"),
                  ).join(""),
                );
                changed();
              }}
            >
              <RefreshCw size={12} />
              Generate
            </button>
            <button
              type="button"
              disabled={!secret}
              aria-label={`Reveal webhook secret for ${repo.name}`}
              aria-pressed={reveal}
              onClick={() => setReveal((v) => !v)}
            >
              {reveal ? <EyeOff size={12} /> : <Eye size={12} />}
            </button>
          </span>
        </label>
      </div>
      <div className="policy-toggles">
        {[
          { label: "Auto-commit", value: auto, set: setAuto },
          { label: "Open PR", value: pr, set: setPr },
          { label: "Manual approval", value: manual, set: setManual },
        ].map((t) => (
          <label key={t.label}>
            <input
              type="checkbox"
              checked={t.value}
              disabled={disabled}
              onChange={(e) => {
                t.set(e.target.checked);
                changed();
              }}
            />
            <span className="toggle-track">
              <i />
            </span>
            {t.label}
          </label>
        ))}
      </div>
      <p className="repository-policy-caption">
        Repair policy applies to the default branch. Repository branch
        protection is managed by your Git provider.
      </p>
      {saveError && (
        <div className="repository-save-error" role="alert">
          {saveError}
        </div>
      )}
      <div className="repository-foot">
        <span>
          <ShieldCheck size={11} />
          HMAC-SHA256 verified pushes
        </span>
        <button type="submit" disabled={disabled || !dirty}>
          <Check size={11} />
          Save settings
        </button>
      </div>
    </form>
  );
}
