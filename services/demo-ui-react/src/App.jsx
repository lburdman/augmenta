import { useState } from 'react';
import './index.css';

function App() {
  const [inputText, setInputText] = useState('My name is John Doe and my email is j.doe@example.com. Please contact me!');
  const [isProcessing, setIsProcessing] = useState(false);
  const [results, setResults] = useState(null);
  const [auditLog, setAuditLog] = useState([]);
  const [gatewayProof, setGatewayProof] = useState(null);

  const INGEST_URL = import.meta.env.VITE_INGESTION_URL || 'http://localhost:8080/ingest/webhook/demo';
  const AUDIT_URL = import.meta.env.VITE_AUDIT_URL || 'http://localhost:8080/admin/audit';
  const GATEWAY_URL = import.meta.env.VITE_GATEWAY_URL || 'http://localhost:7001/last';

  const fetchAuditLog = async (requestId) => {
    try {
      const res = await fetch(`${AUDIT_URL}?requestId=${requestId}`);
      if (!res.ok) throw new Error('Failed to fetch audit log');
      const data = await res.json();
      setAuditLog(data || []);
    } catch (e) {
      console.error(e);
    }
  };

  const fetchGatewayProof = async () => {
    try {
      const res = await fetch(GATEWAY_URL);
      if (!res.ok) throw new Error('Failed to fetch gateway proof');
      const data = await res.json();
      setGatewayProof(data);
    } catch (e) {
      console.error(e);
    }
  };

  const handleRunPipeline = async () => {
    setIsProcessing(true);
    setResults(null);
    setAuditLog([]);
    setGatewayProof(null);

    try {
      // 1. Send data to ingestion webhook
      const reqStart = Date.now();
      const res = await fetch(INGEST_URL, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Tenant-ID': 'tenantA',
        },
        body: JSON.stringify({ text: inputText }),
      });

      if (!res.ok) throw new Error(`HTTP error! status: ${res.status}`);

      const data = await res.json();
      setResults(data);

      // Wait a tiny bit for async logs/forwarders fully flush
      setTimeout(async () => {
        if (data.requestId) {
          await fetchAuditLog(data.requestId);
          await fetchGatewayProof();
        }
      }, 500);

    } catch (error) {
      alert(`Pipeline failed: ${error.message}`);
    } finally {
      setIsProcessing(false);
    }
  };

  return (
    <div className="container">
      <header>
        <h1>Augmenta PII Vault Demo</h1>
        <p className="subtitle">Visualize the zero-trust tokenization and rehydration pipeline</p>
      </header>

      <main>
        <div className="card input-card">
          <h2>1. Input Data</h2>
          <textarea
            value={inputText}
            onChange={(e) => setInputText(e.target.value)}
            placeholder="Enter text containing PII..."
            rows={4}
          />
          <button
            onClick={handleRunPipeline}
            disabled={isProcessing || !inputText.trim()}
            className={isProcessing ? 'processing' : ''}
          >
            {isProcessing ? 'Processing Pipeline...' : 'Run Pipeline'}
          </button>
        </div>

        {results && (
          <div className="results-grid">
            <div className="card">
              <h2>2. Anonymization Phase</h2>
              <div className="label">Anonymized Text (Sent to LLM / Gateway)</div>
              <div className="box code-box secure">
                {results.anonymized_text}
              </div>
            </div>

            <div className="card">
              <h2>3. LLM Gateway Response</h2>
              <div className="label">Mock LLM Output (Tokens Preserved)</div>
              <div className="box code-box neutral">
                {results.llm_output}
              </div>
            </div>

            <div className="card full-width">
              <h2>4. Rehydration Phase</h2>
              <div className="label">Final Output (Re-injected in Memory)</div>
              <div className="box code-box success">
                {results.rehydrated_output || 'Skipped or Failed'}
              </div>
            </div>
          </div>
        )}

        <div className="audit-proof-container">
          <div className="card">
            <h2>Audit Trail</h2>
            {auditLog.length > 0 ? (
              <table className="audit-table">
                <thead>
                  <tr>
                    <th>Step</th>
                    <th>Outcome</th>
                    <th>Reason</th>
                    <th>Latency</th>
                  </tr>
                </thead>
                <tbody>
                  {auditLog.map((log, i) => (
                    <tr key={i} className={log.outcome}>
                      <td className="mono">{log.step}</td>
                      <td>
                        <span className={`badge ${log.outcome}`}>{log.outcome}</span>
                      </td>
                      <td className="mono">{log.reason_code || '-'}</td>
                      <td>{log.latency_ms}ms</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <p className="empty">No audit events generated yet.</p>
            )}
          </div>

          <div className="card">
            <h2>Gateway Proof</h2>
            {gatewayProof ? (
              <div className="proof-box">
                <div className="proof-row">
                  <span className="label">Request ID:</span>
                  <span className="mono">{gatewayProof.requestId}</span>
                </div>
                <div className="proof-row">
                  <span className="label">Prompt Hash:</span>
                  <span className="mono hash">{gatewayProof.prompt_hash}</span>
                </div>
                <div className="alert-box">
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"></path></svg>
                  <p><strong>Proof:</strong> Raw PII is never sent to the LLM Gateway. Only anonymized tokens and hashed prompt metadata are visible to external providers.</p>
                </div>
              </div>
            ) : (
              <p className="empty">Waiting for downstream gateway intercept...</p>
            )}
          </div>
        </div>
      </main>
    </div>
  );
}

export default App;
