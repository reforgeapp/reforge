import { useState, type FormEvent } from 'react'
import { Button, Dialog } from '../components/Accessible'
import { discoveryAPI, type Finding } from '../api/discovery'

export function BugReportDialog({ orgID, csrf, repositoryID, repositories, onClose, onReported }: { orgID: string; csrf: string; repositoryID: string; repositories: Array<{ id: string; name: string }>; onClose: () => void; onReported: (finding: Finding) => void }) {
  const [repo, setRepo] = useState(repositoryID)
  const [observed, setObserved] = useState('')
  const [expected, setExpected] = useState('')
  const [url, setURL] = useState('')
  const [screenshot, setScreenshot] = useState('')
  const [errorText, setErrorText] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (event: FormEvent) => { event.preventDefault(); setBusy(true); setError(''); try { onReported(await discoveryAPI.reportBug(orgID, { repository_id: repo, kind: 'defect', observed, expected, url, screenshot_url: screenshot, error: errorText }, csrf)); onClose() } catch (reason) { setError(reason instanceof Error ? reason.message : 'Report could not be saved') } finally { setBusy(false) } }
  return <Dialog open title="Fix this" onClose={onClose}><form className="form-stack" onSubmit={submit}><label>Repository<select required value={repo} onChange={event => setRepo(event.target.value)}><option value="">Choose repository</option>{repositories.map(repository => <option key={repository.id} value={repository.id}>{repository.name}</option>)}</select></label><label>What stopped working?<textarea required maxLength={2000} value={observed} onChange={event => setObserved(event.target.value)} placeholder="Checkout button does nothing on mobile" /></label><label>What should happen?<textarea required maxLength={2000} value={expected} onChange={event => setExpected(event.target.value)} placeholder="Tap checkout to continue to payment" /></label><details><summary>URL, screenshot or error</summary><label>Page URL<input type="url" maxLength={2048} value={url} onChange={event => setURL(event.target.value)} /></label><label>Screenshot URL<input type="url" maxLength={2048} value={screenshot} onChange={event => setScreenshot(event.target.value)} /></label><label>Error text<textarea maxLength={4096} value={errorText} onChange={event => setErrorText(event.target.value)} /></label></details><p className="table-meta">Repairs existing behaviour. New features need a separate development request.</p>{error && <p className="error-text" role="alert">{error}</p>}<Button className="button button-primary" disabled={busy || !csrf || !repo}>{busy ? 'Reporting…' : 'Fix this'}</Button></form></Dialog>
}
