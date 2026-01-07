<script>
window.addEventListener("load", () => {
   const params = new URLSearchParams(window.location.search);
   const yymmdd = (params.get("date") || "").trim();
   const B = (() => {
      const date = new Date(yymmdd);
      if (isNaN(date.getTime())) {
         return new Date("1998/09/08");
      }
      return date;
   })();

   const D  = new Date() - B;
   const A  = D / (1000 * 60 * 60 * 24 * 365.25);
   const L  = 78.4;
   const H  = (A / L) * 24;
   const HH = Math.floor(H);
   const M  = (H - HH) * 60;
   const MM = Math.floor(M);
   const SS = Math.floor((M - MM) * 60);

   const pad   = (n) => n.toString().padStart(2, '0');
   const short = `${pad(HH % 12 || 12)}:${pad(MM)} ${HH >= 12 ? 'PM' : 'AM'}`;
   const full  = `${pad(HH % 12 || 12)}:${pad(MM)}:${pad(SS)} ${HH >= 12 ? 'PM' : 'AM'}`;

   const time = document.getElementById("time");
   time.innerHTML = short;

   document.title = document.title.replace("Time", full);
});
</script>
