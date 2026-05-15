// SPDX-Licence-Identifier: EUPL-1.2

// webview_extra.go — additional webview_* tools beyond the
// minimum-viable set in tools.go. All composed as JS one-liners
// dispatched through s.eval() against the named window.

package bridge

import (

	core "dappco.re/go"
)

// toolWebviewHover dispatches mouseover + mouseenter on the
// element matching `selector`. params: { selector, window? }
func (s *Service) toolWebviewHover(ctx core.Context, params map[string]any) map[string]any {
	sel := jsonLit(paramString(params, "selector", ""))
	return s.eval(ctx, paramString(params, "window", DefaultWindow),
		core.Sprintf(`var el=document.querySelector(%s);if(!el)throw new Error("element not found: "+%s);['mouseover','mouseenter','pointerover','pointerenter'].forEach(function(t){el.dispatchEvent(new MouseEvent(t,{bubbles:true,cancelable:true,view:window}));});return {hovered:true,tag:el.tagName.toLowerCase()};`, sel, sel))
}

// toolWebviewType sets an input/textarea's value and dispatches
// input + change events so framework bindings (Lit @input, React
// onChange) see the update. params: { selector, value, window? }
func (s *Service) toolWebviewType(ctx core.Context, params map[string]any) map[string]any {
	sel := jsonLit(paramString(params, "selector", ""))
	val := jsonLit(paramString(params, "value", ""))
	return s.eval(ctx, paramString(params, "window", DefaultWindow),
		core.Sprintf(`var el=document.querySelector(%s);if(!el)throw new Error("element not found: "+%s);if('value' in el){el.value=%s;}else{el.textContent=%s;}el.dispatchEvent(new Event('input',{bubbles:true}));el.dispatchEvent(new Event('change',{bubbles:true}));return {typed:true,tag:el.tagName.toLowerCase(),value:%s};`, sel, sel, val, val, val))
}

// toolWebviewCheck sets a checkbox or radio's checked state.
// params: { selector, checked, window? }
func (s *Service) toolWebviewCheck(ctx core.Context, params map[string]any) map[string]any {
	sel := jsonLit(paramString(params, "selector", ""))
	checked := paramBool(params, "checked", true)
	checkedJS := "true"
	if !checked {
		checkedJS = "false"
	}
	return s.eval(ctx, paramString(params, "window", DefaultWindow),
		core.Sprintf(`var el=document.querySelector(%s);if(!el)throw new Error("element not found: "+%s);if(el.type!=='checkbox'&&el.type!=='radio')throw new Error("not a checkbox/radio: "+el.tagName);el.checked=%s;el.dispatchEvent(new Event('change',{bubbles:true}));return {checked:el.checked,tag:el.tagName.toLowerCase()};`, sel, sel, checkedJS))
}

// toolWebviewSelect picks an option in a <select>. value matches
// either the option's value attr or its text content. params:
// { selector, value, window? }
func (s *Service) toolWebviewSelect(ctx core.Context, params map[string]any) map[string]any {
	sel := jsonLit(paramString(params, "selector", ""))
	val := jsonLit(paramString(params, "value", ""))
	return s.eval(ctx, paramString(params, "window", DefaultWindow),
		core.Sprintf(`var el=document.querySelector(%s);if(!el)throw new Error("element not found: "+%s);if(el.tagName!=='SELECT')throw new Error("not a select: "+el.tagName);var target=%s;var matched=Array.from(el.options).find(function(o){return o.value===target||o.text===target;});if(!matched)throw new Error("option not found: "+target);el.value=matched.value;el.dispatchEvent(new Event('change',{bubbles:true}));return {selected:matched.value,text:matched.text};`, sel, sel, val))
}

// toolWebviewScroll scrolls the page so the matched element is in
// view (or to absolute coords when only x/y given). params:
// { selector?, x?, y?, behavior?, window? }
func (s *Service) toolWebviewScroll(ctx core.Context, params map[string]any) map[string]any {
	sel := core.Trim(paramString(params, "selector", ""))
	behavior := paramString(params, "behavior", "smooth")
	bJS := jsonLit(behavior)
	if sel != "" {
		selJS := jsonLit(sel)
		return s.eval(ctx, paramString(params, "window", DefaultWindow),
			core.Sprintf(`var el=document.querySelector(%s);if(!el)throw new Error("element not found: "+%s);el.scrollIntoView({behavior:%s,block:'center',inline:'nearest'});return {scrolledTo:%s};`, selJS, selJS, bJS, selJS))
	}
	x := paramInt(params, "x", 0)
	y := paramInt(params, "y", 0)
	return s.eval(ctx, paramString(params, "window", DefaultWindow),
		core.Sprintf(`window.scrollTo({top:%d,left:%d,behavior:%s});return {scrolledTo:{x:%d,y:%d}};`, y, x, bJS, x, y))
}

// toolWebviewDOMTree returns a depth-limited DOM tree starting
// from `selector` (or document.body when omitted). params:
// { selector?, maxDepth?, window? }
func (s *Service) toolWebviewDOMTree(ctx core.Context, params map[string]any) map[string]any {
	root := jsonLit(paramString(params, "selector", "body"))
	depth := paramInt(params, "maxDepth", 6)
	return s.eval(ctx, paramString(params, "window", DefaultWindow),
		core.Sprintf(`function walk(el,d){if(!el||d<0)return null;var out={tag:el.tagName?el.tagName.toLowerCase():'#text',id:el.id||undefined,classes:el.classList?Array.from(el.classList):undefined};if(d===0)return out;var kids=[];for(var i=0;i<el.children.length;i++){var c=walk(el.children[i],d-1);if(c)kids.push(c);}if(kids.length)out.children=kids;return out;}var root=document.querySelector(%s);if(!root)throw new Error("root not found: "+%s);return walk(root,%d);`, root, root, depth))
}

// toolWebviewSource returns the full document.documentElement.outerHTML.
// params: { window? }
func (s *Service) toolWebviewSource(ctx core.Context, params map[string]any) map[string]any {
	return s.eval(ctx, paramString(params, "window", DefaultWindow),
		`return document.documentElement.outerHTML;`)
}

// toolWebviewComputedStyle returns getComputedStyle entries for
// the matched element, optionally narrowed by props list. params:
// { selector, props?, window? }
func (s *Service) toolWebviewComputedStyle(ctx core.Context, params map[string]any) map[string]any {
	sel := jsonLit(paramString(params, "selector", ""))
	propsRaw, _ := params["props"].([]any)
	propsJS := "null"
	if len(propsRaw) > 0 {
		// Build a JS array literal from the requested prop names.
		var b string
		b += "["
		for i, p := range propsRaw {
			if i > 0 {
				b += ","
			}
			b += jsonLit(core.Sprintf("%v", p))
		}
		b += "]"
		propsJS = b
	}
	return s.eval(ctx, paramString(params, "window", DefaultWindow),
		core.Sprintf(`var el=document.querySelector(%s);if(!el)throw new Error("element not found: "+%s);var cs=window.getComputedStyle(el);var picks=%s;var out={};if(picks){picks.forEach(function(p){out[p]=cs.getPropertyValue(p);});}else{for(var i=0;i<cs.length;i++){var k=cs.item(i);out[k]=cs.getPropertyValue(k);}}return out;`, sel, sel, propsJS))
}

// toolWebviewElementInfo returns a rich descriptor for the matched
// element — tag, attrs, bounds, computed display, text snippet.
// params: { selector, window? }
func (s *Service) toolWebviewElementInfo(ctx core.Context, params map[string]any) map[string]any {
	sel := jsonLit(paramString(params, "selector", ""))
	return s.eval(ctx, paramString(params, "window", DefaultWindow),
		core.Sprintf(`var el=document.querySelector(%s);if(!el)throw new Error("element not found: "+%s);var r=el.getBoundingClientRect();var cs=window.getComputedStyle(el);var attrs={};for(var i=0;i<el.attributes.length;i++){var a=el.attributes[i];attrs[a.name]=a.value;}return {tag:el.tagName.toLowerCase(),id:el.id||null,classes:Array.from(el.classList),attrs:attrs,bounds:{x:r.x,y:r.y,w:r.width,h:r.height},display:cs.display,visibility:cs.visibility,opacity:cs.opacity,text:(el.textContent||'').slice(0,500),childCount:el.children.length,hasFocus:document.activeElement===el};`, sel, sel))
}

// toolWebviewHighlight outlines the matched element with a
// temporary box-shadow + outline for the user to visually locate.
// params: { selector, duration?, window? } — duration in ms, default 2000
func (s *Service) toolWebviewHighlight(ctx core.Context, params map[string]any) map[string]any {
	sel := jsonLit(paramString(params, "selector", ""))
	dur := paramInt(params, "duration", 2000)
	return s.eval(ctx, paramString(params, "window", DefaultWindow),
		core.Sprintf(`var el=document.querySelector(%s);if(!el)throw new Error("element not found: "+%s);var prev={outline:el.style.outline,outlineOffset:el.style.outlineOffset,boxShadow:el.style.boxShadow};el.style.outline='3px solid rgba(167,139,250,0.95)';el.style.outlineOffset='2px';el.style.boxShadow='0 0 0 9999px rgba(0,0,0,0.45)';setTimeout(function(){el.style.outline=prev.outline;el.style.outlineOffset=prev.outlineOffset;el.style.boxShadow=prev.boxShadow;},%d);el.scrollIntoView({behavior:'smooth',block:'center'});return {highlighted:true,tag:el.tagName.toLowerCase()};`, sel, sel, dur))
}

// toolWebviewConsoleClear empties the bridge's server-side console
// ring buffer. Mirrors Service.ClearConsole but exposed as an MCP
// tool for parity with the core/ide bridge surface.
func (s *Service) toolWebviewConsoleClear() map[string]any {
	s.ClearConsole()
	return map[string]any{"ok": true, "cleared": "console"}
}

// toolWebviewPerformance returns performance.timing + memory (where
// supported) for the window. params: { window? }
func (s *Service) toolWebviewPerformance(ctx core.Context, params map[string]any) map[string]any {
	return s.eval(ctx, paramString(params, "window", DefaultWindow),
		`var t=performance.timing?{...performance.timing.toJSON()}:null;var nav=performance.getEntriesByType('navigation')[0]||null;var mem=(performance.memory)?{usedJSHeapSize:performance.memory.usedJSHeapSize,totalJSHeapSize:performance.memory.totalJSHeapSize,jsHeapSizeLimit:performance.memory.jsHeapSizeLimit}:null;return {timing:t,navigation:nav,memory:mem,now:performance.now()};`)
}
