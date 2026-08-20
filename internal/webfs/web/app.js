// Frontend app for the railway interlocking engine. Native JS, no build step.
// Talks to the JSON API; renders a live yard diagram. The smoke-test covers the
// same create-station → build-elements → define-route → establish → train move
// → view flow that a human operator would walk through.

const API = "";
const $ = (id) => document.getElementById(id);

async function api(method, path, body) {
	const opt = { method, headers: {} };
	if (body !== undefined) {
		opt.headers["Content-Type"] = "application/json";
		opt.body = JSON.stringify(body);
	}
	const res = await fetch(API + path, opt);
	const text = await res.text();
	let data = null;
	try { data = text ? JSON.parse(text) : null; } catch { data = { raw: text }; }
	if (!res.ok) {
		const msg = (data && data.error) || ("HTTP " + res.status);
		throw new Error(msg);
	}
	return data;
}

function opt(select, value, label) {
	const o = document.createElement("option");
	o.value = value; o.textContent = label;
	select.appendChild(o);
}

function reloadSelect(select, items, valField, labelField) {
	select.innerHTML = "";
	items.forEach((it) => opt(select, it[valField], it[labelField]));
}

async function refreshStations() {
	const d = await api("GET", "/stations");
	const sts = d.stations || [];
	reloadSelect($("stationSelect"), sts, "id", "name");
	$("swSection").innerHTML = "";
	$("rtSource").innerHTML = "";
	$("rtApproach").innerHTML = "";
	$("moveSection").innerHTML = "";
	await refreshStationView();
}

async function refreshStationView() {
	const sid = $("stationSelect").value;
	if (!sid) { $("diagram").innerHTML = "（选择车站后显示）"; return; }
	try {
		const v = await api("GET", "/stations/" + sid);
		renderDiagram(v);
		reloadSelect($("swSection"), v.sections, "id", "name");
		const homes = (v.signals || []).filter((s) => s.home);
		reloadSelect($("rtSource"), homes, "id", "name");
		reloadSelect($("rtApproach"), (v.sections || []).filter((s) => s.kind === "approach"), "id", "name");
		reloadSelect($("moveSection"), v.sections, "id", "name");
		const routes = v.routes || [];
		$("routeSelect").innerHTML = "";
		routes.forEach((r) => opt($("routeSelect"), r.id, r.code + " (" + r.state + ")"));
		const trains = v.trains || [];
		$("trainSelect").innerHTML = "";
		trains.forEach((t) => opt($("trainSelect"), t.id, t.code));
	} catch (e) { showError(e); }
}

function renderDiagram(v) {
	const el = $("diagram");
	el.innerHTML = "";
	const card = (title, lines) => {
		const d = document.createElement("div");
		d.className = "card";
		d.innerHTML = `<div class="t">${title}</div>${lines.join("<br/>")}`;
		el.appendChild(d);
	};
	(v.switches || []).forEach((s) => {
		const pos = s.current_position === 0 ? "定位" : "反位";
		const occ = s.locked ? `<span class="badge locked">锁闭(${s.locked_by_route})</span>` : `<span class="badge free">空闲</span>`;
		const ind = s.has_indication ? "" : `<span class="badge occ">失表示</span>`;
		card("道岔 " + s.name, [
			`<span class="k">位置</span> ${pos} ${occ} ${ind}`,
		]);
	});
	(v.signals || []).forEach((s) => {
		card("信号 " + s.name, [
			`<span class="k">方向</span> ${s.direction}`,
			`显示 <span class="aspect ${s.aspect}"></span> (${s.aspect})`,
		]);
	});
	(v.sections || []).forEach((s) => {
		const occ = s.occupied ? `<span class="badge occ">占用(${s.occupied_by_train})</span>` : `<span class="badge free">空闲</span>`;
		const lk = s.locked ? `<span class="badge locked">锁闭(${s.locked_by_route})</span>` : "";
		card("区段 " + s.name, [`<span class="k">${s.kind}</span> ${occ} ${lk}`]);
	});
	(v.routes || []).forEach((r) => {
		card("进路 " + r.code, [
			`<span class="k">${r.kind}</span> 状态 ${r.state}`,
			r.train_id ? `车 ${r.train_id}` : "",
		]);
	});
}

function showError(e) { $("raw").textContent = String(e.message || e); }

function bind(id, fn) { $(id).addEventListener("click", async () => { try { await fn(); } catch (e) { showError(e); } }); }

bind("btnStation", async () => {
	await api("POST", "/stations", { code: $("stCode").value, name: $("stName").value });
	await refreshStations();
});
bind("btnSection", async () => {
	const sid = $("stationSelect").value;
	await api("POST", `/stations/${sid}/sections`, { name: $("scName").value, kind: $("scKind").value });
	await refreshStationView();
});
bind("btnSwitch", async () => {
	const sid = $("stationSelect").value;
	await api("POST", `/stations/${sid}/switches`, {
		name: $("swName").value,
		normal_position: parseInt($("swNormal").value, 10),
		section_id: $("swSection").value,
	});
	await refreshStationView();
});
bind("btnSignal", async () => {
	const sid = $("stationSelect").value;
	await api("POST", `/stations/${sid}/signals`, {
		name: $("sgName").value,
		direction: $("sgDir").value,
		home: $("sgHome").checked,
	});
	await refreshStationView();
});
bind("btnRoute", async () => {
	// Build a simple route from the current selections: straight through all
	// sections in order (a helper for the demo; a real operator picks a
	// subset).
	const sid = $("stationSelect").value;
	const v = await api("GET", `/stations/${sid}`);
	const secs = (v.sections || []).filter((s) => s.kind !== "approach").slice(0, 3);
	const swPos = (v.switches || []).map((s) => ({ switch_id: s.id, required_position: s.normal_position }));
	await api("POST", "/routes", {
		station_id: sid,
		code: $("rtCode").value,
		kind: $("rtKind").value,
		source_signal_id: $("rtSource").value,
		terminal: $("rtTerminal").value || (secs[secs.length - 1] || {}).id || "",
		approach_section_id: $("rtApproach").value,
		switch_positions: swPos,
		sections: secs.map((s, i) => ({ section_id: s.id, seq: i })),
	});
	await refreshStationView();
});
bind("btnEstablish", async () => {
	const rid = $("routeSelect").value;
	await api("POST", `/routes/${rid}/establish`);
	await refreshStationView();
});
bind("btnCancel", async () => {
	const rid = $("routeSelect").value;
	await api("POST", `/routes/${rid}/cancel`);
	await refreshStationView();
});
bind("btnFault", async () => {
	const rid = $("routeSelect").value;
	await api("POST", `/routes/${rid}/fault-unlock`, {});
	await refreshStationView();
});
bind("btnTrain", async () => {
	const sid = $("stationSelect").value;
	await api("POST", "/trains", { code: $("trCode").value, station_id: sid });
	await refreshStationView();
});
bind("btnOccupy", async () => {
	const tid = $("trainSelect").value;
	await api("POST", `/trains/${tid}/occupy`, { section_id: $("moveSection").value });
	await refreshStationView();
});
bind("btnClear", async () => {
	const tid = $("trainSelect").value;
	await api("POST", `/trains/${tid}/clear`, { section_id: $("moveSection").value });
	await refreshStationView();
});

refreshStations().catch(showError);
