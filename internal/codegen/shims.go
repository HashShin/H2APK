package codegen

import (
	"h2apk/internal/types"
	"fmt"
	"strings"
)

// clipShimScript returns a <script> block that polyfills clipboard access
// and intercepts download-anchor clicks via the H2AClip / H2AFile bridges.
func clipShimScript() string {
	return `
  <script>
(function(){
  if(typeof H2AClip==='undefined')return;
  if(!navigator.clipboard)navigator.clipboard={};
  navigator.clipboard.readText=function(){
    try{return Promise.resolve(H2AClip.readText());}catch(e){return Promise.reject(e);}
  };
  navigator.clipboard.writeText=function(t){
    try{H2AClip.writeText(String(t));return Promise.resolve();}catch(e){return Promise.reject(e);}
  };
})();
(function(){
  function handleDownloadAnchor(a){
    var href=a.href||'';
    var filename=a.getAttribute('download')||'download';
    if(href.startsWith('data:')){
      var comma=href.indexOf(',');
      if(comma<0)return false;
      var meta=href.substring(5,comma);
      var b64part=href.substring(comma+1);
      var mime=meta.split(';')[0]||'application/octet-stream';
      var b64=meta.indexOf('base64')>=0?b64part:btoa(decodeURIComponent(b64part.replace(/\+/g,' ')));
      if(window.H2AFile){H2AFile.saveBase64File(b64,filename,mime);return true;}
    }
    if(href.startsWith('blob:')){
      fetch(href).then(function(r){return r.blob();}).then(function(b){
        var mime=b.type||'application/octet-stream';
        var fr=new FileReader();
        fr.onload=function(){
          var res=fr.result;
          var b64=res.indexOf(',')>=0?res.split(',')[1]:res;
          if(window.H2AFile)H2AFile.saveBase64File(b64,filename,mime);
        };
        fr.readAsDataURL(b);
      }).catch(function(e){console.error('blob dl',e);});
      return true;
    }
    return false;
  }
  document.addEventListener('click',function(e){
    var el=e.target;
    while(el&&el.tagName!=='A')el=el.parentElement;
    if(!el||!el.hasAttribute('download'))return;
    if(handleDownloadAnchor(el)){e.preventDefault();e.stopPropagation();}
  },true);
})();
(function(){
  var _origClick=HTMLInputElement.prototype.click;
  HTMLInputElement.prototype.click=function(){
    if(this.type==='file'){
      var prev=this.style.cssText;
      var wasHidden=(this.offsetParent===null||this.style.display==='none'||this.style.visibility==='hidden'||this.getAttribute('type')==='file'&&getComputedStyle(this).display==='none');
      if(wasHidden){
        this.style.setProperty('position','fixed','important');
        this.style.setProperty('top','0','important');
        this.style.setProperty('left','0','important');
        this.style.setProperty('width','1px','important');
        this.style.setProperty('height','1px','important');
        this.style.setProperty('opacity','0','important');
        this.style.setProperty('display','block','important');
        this.style.setProperty('visibility','visible','important');
        this.style.setProperty('z-index','-9999','important');
      }
      _origClick.call(this);
      if(wasHidden){var el=this;setTimeout(function(){el.style.cssText=prev;},500);}
    } else {
      _origClick.call(this);
    }
  };
})();
  </script>`
}

// notifShimScript returns a <script> block that polyfills the Web Notification API
// using the H2A (NotificationHelper) bridge.
func notifShimScript() string {
	return `
  <script>
(function(){
  if(typeof Notification!=='undefined')return;
  if(typeof H2A==='undefined'||!H2A.showNotification)return;
  var p=H2A.getNotificationPermission(),cbs=[];
  window.Notification=function(t,o){
    if(p==='granted')H2A.showNotification(t,(o&&o.body)||'');
  };
  Object.defineProperty(Notification,'permission',{get:function(){return p;}});
  Notification.requestPermission=function(){
    return new Promise(function(r){
      if(p==='granted'){r('granted');return;}
      cbs.push(r);H2A.requestNotificationPermission();
      var n=0,i=setInterval(function(){
        p=H2A.getNotificationPermission();
        if(p==='granted'||++n>60){
          clearInterval(i);
          cbs.forEach(function(c){c(p==='granted'?'granted':'denied');});
          cbs=[];
        }
      },500);
    });
  };
})();
  </script>`
}

// shareShimScript returns a <script> block that polyfills navigator.share
// using the H2AShare bridge.
func shareShimScript() string {
	return `
  <script>
(function(){
  if(navigator.share)return;
  if(typeof H2AShare==='undefined')return;
  navigator.share=function(d){
    d=d||{};
    return new Promise(function(res){
      H2AShare.share(d.title||'',d.text||'',d.url||'');
      res();
    });
  };
  navigator.canShare=function(d){ return !(d&&d.files&&d.files.length); };
})();
  </script>`
}

// speechShimScript returns a <script> block that patches or polyfills the
// SpeechSynthesis API using the H2ATTS bridge.
func speechShimScript() string {
	return `
  <script>
(function(){
  if(window.speechSynthesis&&window.SpeechSynthesisUtterance){
    // Native API present — patch speak() to retry after voices load
    var s=window.speechSynthesis,origSpeak=s.speak.bind(s);
    s.speak=function(u){
      if(s.getVoices().length===0){
        var done=false;
        var fire=function(){ if(done)return; done=true; origSpeak(u); };
        s.addEventListener('voiceschanged',fire,{once:true});
        setTimeout(fire,250);
      } else { origSpeak(u); }
    };
    return;
  }
  // Native API absent — polyfill via Android TTS bridge
  if(typeof H2ATTS==='undefined')return;
  var fakeVoice={name:'Android TTS',lang:navigator.language||'en',localService:true,default:true,voiceURI:'android'};
  window.SpeechSynthesisUtterance=function(text){
    this.text=text||'';
    this.lang=navigator.language||'en';
    this.rate=1;this.pitch=1;this.volume=1;
    this.voice=fakeVoice;
    this.onstart=null;this.onend=null;this.onerror=null;
  };
  window.speechSynthesis={
    speaking:false,paused:false,pending:false,
    getVoices:function(){ return [fakeVoice]; },
    speak:function(u){
      this.speaking=true;
      if(u.onstart) try{u.onstart({});}catch(e){}
      H2ATTS.speak(u.text||'');
      var self=this;
      setTimeout(function(){
        self.speaking=false;
        if(u.onend) try{u.onend({});}catch(e){}
      }, Math.max(500, (u.text||'').length*60));
    },
    cancel:function(){ H2ATTS.stop(); this.speaking=false; },
    pause:function(){},
    resume:function(){}
  };
  // Fire voiceschanged so callers waiting on it unblock
  setTimeout(function(){
    var e=new Event('voiceschanged');
    window.speechSynthesis.dispatchEvent&&window.speechSynthesis.dispatchEvent(e);
  },100);
})();
  </script>`
}

// shimBlocks assembles all shim scripts appropriate for the given request.
func shimBlocks(req types.BuildRequest) string {
	s := clipShimScript()
	if req.NotifPermission {
		s = notifShimScript() + s
	}
	s = shareShimScript() + speechShimScript() + s
	return s
}

// injectShims inserts shim scripts into an HTML document before </body>,
// </html>, or </head> (in that order of preference).
func InjectShims(htmlContent string, req types.BuildRequest) string {
	shims := shimBlocks(req)
	lower := strings.ToLower(htmlContent)
	if i := strings.LastIndex(lower, "</body>"); i != -1 {
		return htmlContent[:i] + shims + "\n" + htmlContent[i:]
	}
	if i := strings.LastIndex(lower, "</html>"); i != -1 {
		return htmlContent[:i] + shims + "\n" + htmlContent[i:]
	}
	if i := strings.LastIndex(lower, "</head>"); i != -1 {
		return htmlContent[:i] + shims + "\n" + htmlContent[i:]
	}
	return htmlContent + "\n" + shims
}

// wrapHTML builds a complete HTML document from the build request,
// embedding CSS, JS, and all shim scripts.
func WrapHTML(req types.BuildRequest) string {
	css := ""
	if req.CSS != "" {
		css += "\n  " + req.CSS
	}
	css = "\n  <style>\n  body{margin:0;color:#ffffff}\n" + css + "\n  </style>"
	notifShim := ""
	if req.NotifPermission {
		notifShim = notifShimScript()
	}
	clipShim := clipShimScript()
	shareShim := shareShimScript()
	speechShim := speechShimScript()
	js := ""
	if req.JS != "" {
		js = "\n  <script>\n" + req.JS + "\n  </script>"
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1.0,user-scalable=yes,maximum-scale=5.0">
  <title>%s</title>%s%s%s%s%s
</head>
<body>
%s%s
</body>
</html>`, req.AppName, css, notifShim, shareShim, speechShim, clipShim, req.HTML, js)
}
