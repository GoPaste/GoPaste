"""BGM 节奏分析（music-beat-sync §1–3）：网格拟合 + 0.5x/1x/2x 候选验收 + 三分类鼓点 + RMS 结构。
用法：.venv/bin/python analysis/analyze_bgm.py public/audio/bgm/house-vibez.mp3
产物：analysis/beat_data.json、analysis/grid_drift.json（时刻全部为浮点秒，不预取整）
"""
import json
import sys
from pathlib import Path

import librosa
import numpy as np
from scipy.signal import butter, sosfilt

src = Path(sys.argv[1])
out = Path(__file__).parent
y, sr = librosa.load(src, sr=None, mono=True)
dur = len(y) / sr
y_perc = librosa.effects.hpss(y)[1]

_, beats = librosa.beat.beat_track(y=y_perc, sr=sr, tightness=400, units="time")
i = np.arange(len(beats))
(T, t0), *_ = np.linalg.lstsq(np.vstack([i, np.ones_like(i)]).T, beats, rcond=None)
while t0 - T > 0:
    t0 -= T
resid = beats - (t0 + np.round((beats - t0) / T) * T)


def band_env(sig, lo, hi):
    sos = butter(4, [lo, hi], btype="band", fs=sr, output="sos")
    env = librosa.onset.onset_strength(y=sosfilt(sos, sig), sr=sr)
    return env / (env.max() + 1e-9)


times = librosa.times_like(band_env(y_perc, 40, 160), sr=sr)
bands = {"kick": (40, 160), "snare": (1000, 3000), "hihat": (6000, min(14000, sr / 2 - 100))}
envs = {k: band_env(y_perc, *v) for k, v in bands.items()}
full_env = librosa.onset.onset_strength(y=y_perc, sr=sr)
onsets = librosa.onset.onset_detect(onset_envelope=full_env, sr=sr, units="time", backtrack=False)


def grid_score(Tc, t0c):
    n = int((dur - t0c) / Tc)
    g = t0c + np.arange(n) * Tc
    d = np.array([np.min(np.abs(onsets - t)) for t in g])
    hit = d < 0.035
    slope = np.polyfit(g[hit], (np.array([onsets[np.argmin(np.abs(onsets - t))] for t in g[hit]]) - g[hit]), 1)[0] if hit.sum() > 3 else 0
    return {"T": Tc, "bpm": 60 / Tc, "match": float(hit.mean()), "mean_abs_ms": float(d[hit].mean() * 1000),
            "drift_ms_total": float(slope * dur * 1000), "first_beat_on_onset": bool(d[0] < 0.035)}


cands = {"0.5x": grid_score(T * 2, t0), "1x": grid_score(T, t0), "2x": grid_score(T / 2, t0)}
# kick 落在整数拍的比例（判 2x/0.5x 歧义）
kick_peaks = librosa.util.peak_pick(envs["kick"], pre_max=3, post_max=3, pre_avg=5, post_avg=5, delta=0.15, wait=5)
kt = times[kick_peaks]
ph = ((kt - t0) / T) % 1
kick_on_beat = float(np.mean((ph < 0.1) | (ph > 0.9)))

hits = []
for k, env in envs.items():
    pk = librosa.util.peak_pick(env, pre_max=3, post_max=3, pre_avg=5, post_avg=5, delta=0.12, wait=4)
    hits += [{"t": float(times[p]), "s": float(env[p]), "k": k} for p in pk]
hits.sort(key=lambda h: h["t"])

hop = 512
rms = librosa.feature.rms(y=y, hop_length=hop)[0]
rt = librosa.times_like(rms, sr=sr, hop_length=hop)
nbeats = int((dur - t0) / T)
per_beat = []
for n in range(nbeats):
    a, b = t0 + n * T, t0 + (n + 1) * T
    m = (rt >= a) & (rt < b)
    kidx = np.argmin(np.abs(times - a))
    per_beat.append({"n": n, "t": float(a), "rms": float(rms[m].mean()) if m.any() else 0.0,
                     "kick": float(envs["kick"][max(0, kidx - 1):kidx + 2].max()),
                     "snare": float(envs["snare"][max(0, kidx - 1):kidx + 2].max()),
                     "hihat": float(envs["hihat"][max(0, kidx - 1):kidx + 2].max())})

# 每 8 拍一段的结构表
sections = []
for s in range(0, nbeats, 8):
    seg = per_beat[s:s + 8]
    sections.append({"beat": s, "t": seg[0]["t"], "rms": float(np.mean([b["rms"] for b in seg])),
                     "kick": float(np.mean([b["kick"] for b in seg])), "hihat": float(np.mean([b["hihat"] for b in seg]))})

data = {"source": str(src), "duration": dur, "sr": sr, "bpm": float(60 / T), "t0": float(t0), "T": float(T),
        "fit_residual_ms_max": float(np.abs(resid).max() * 1000), "kick_on_integer_beat": kick_on_beat,
        "beats": [float(t0 + n * T) for n in range(nbeats)], "hits": hits, "per_beat": per_beat, "sections": sections}
(out / "beat_data.json").write_text(json.dumps(data, ensure_ascii=False, indent=1))
win = max(cands, key=lambda k: (cands[k]["match"] >= 0.9, -abs(cands[k]["bpm"] - 120) if cands[k]["match"] >= 0.9 else cands[k]["match"]))
(out / "grid_drift.json").write_text(json.dumps({"candidates": cands, "winner": win, "kick_on_integer_beat": kick_on_beat}, indent=1))

print(f"dur={dur:.1f}s BPM={60/T:.3f} t0={t0:.4f} T={T:.5f} resid_max={np.abs(resid).max()*1000:.1f}ms kick_on_beat={kick_on_beat:.2f}")
for k, v in cands.items():
    print(k, {kk: (round(vv, 3) if isinstance(vv, float) else vv) for kk, vv in v.items()})
print("sections (beat, t, rms, kick, hihat):")
for s in sections[:24]:
    print(f"  b{s['beat']:>3} {s['t']:6.2f}s rms={s['rms']:.3f} kick={s['kick']:.2f} hh={s['hihat']:.2f}")
